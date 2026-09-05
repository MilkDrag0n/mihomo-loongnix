package webgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mihomotui/internal/webconfig"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var hashOnce sync.Once
var testHash string

func setup(t *testing.T, handler http.HandlerFunc) *Server {
	t.Helper()
	hashOnce.Do(func() { testHash, _ = webconfig.HashPassword("test-only-password") })
	dir, e := os.MkdirTemp("", "mw-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "m.sock")
	ln, e := net.Listen("unix", socket)
	if e != nil {
		t.Fatal(e)
	}
	if handler == nil {
		handler = func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"success":true,"data":[]}`) }
	}
	up := &http.Server{Handler: handler}
	go up.Serve(ln)
	t.Cleanup(func() { up.Close() })
	app, e := New(webconfig.Config{Listen: "127.0.0.1:19080", PublicURL: "http://127.0.0.1:19080", ManagerSocket: socket, PasswordHash: testHash, SummaryToken: strings.Repeat("s", 32), TestMode: true}, dir)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(app.Close)
	return app
}
func req(app *Server, method, path, body string, auth bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:19080"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://127.0.0.1:19080")
	if auth {
		app.mu.Lock()
		if app.sessions["test"] == nil {
			app.sessions["test"] = &session{CSRF: "csrf", Created: time.Now(), Last: time.Now(), Done: make(chan struct{})}
		}
		app.mu.Unlock()
		r.AddCookie(&http.Cookie{Name: cookieName, Value: "test"})
		r.Header.Set("X-CSRF-Token", "csrf")
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	return w
}
func TestAuthAndInputBoundaries(t *testing.T) {
	var calls atomic.Int32
	app := setup(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"success":true,"data":{}}`)
	})
	for _, p := range []string{"/api/v1/status", "/api/v1/logs/stream", "/api/v1/profiles"} {
		if w := req(app, "GET", p, "", false); w.Code != 401 {
			t.Fatalf("%s: %d", p, w.Code)
		}
	}
	for _, tc := range []struct {
		method, path, body string
		code               int
	}{{"PUT", "/api/v1/tun", "{}", 400}, {"PUT", "/api/v1/tun", `{"enabled":false,"extra":true}`, 400}, {"PUT", "/api/v1/proxy-port", `{"port":1.5}`, 400}, {"PUT", "/api/v1/proxy-port", `{"port":65536}`, 400}, {"POST", "/api/v1/web/stop", "{}", 404}, {"GET", "/api/v1/daemon/info", "", 404}, {"GET", "/api/v1/profiles/foo", "", 404}, {"PUT", "/api/v1/tun", `{"enabled":false} {}`, 400}} {
		w := req(app, tc.method, tc.path, tc.body, true)
		if w.Code != tc.code {
			t.Errorf("%+v got %d %s", tc, w.Code, w.Body.String())
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid request reached manager")
	}
	r := httptest.NewRequest("PUT", "http://127.0.0.1:19080/api/v1/tun", strings.NewReader(`{"enabled":true}`))
	r.AddCookie(&http.Cookie{Name: cookieName, Value: "test"})
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("CSRF not rejected")
	}
	r = httptest.NewRequest("GET", "http://attacker.invalid/api/v1/status", nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: "test"})
	w = httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("host not rejected")
	}
}
func TestSummaryTokenCannotEscape(t *testing.T) {
	app := setup(t, nil)
	for _, p := range []string{"/api/v1/status", "/api/v1/profiles", "/api/v1/auth/session", "/api/v1/web/stop"} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:19080"+p, nil)
		r.Header.Set("Authorization", "Bearer "+app.Config.SummaryToken)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("token escaped: %s %d", p, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "http://internal/api/v1/summary", nil)
	r.Header.Set("Authorization", "Bearer "+app.Config.SummaryToken)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"unknown"`) {
		t.Fatal(w.Body.String())
	}
}
func TestForwardSpecialNamesAndRedaction(t *testing.T) {
	var path string
	app := setup(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		fmt.Fprint(w, `{"success":true,"data":{"core":{"running":true,"detail":"private file"},"active_profile":{"id":"a","name":"example","url":"private subscription"}}}`)
	})
	w := req(app, "PUT", "/api/v1/proxy-groups/中文%2F100%25", `{"name":"node"}`, true)
	if w.Code != 200 || path != "/v1/proxy-groups/中文/100%" {
		t.Fatalf("%d %s", w.Code, path)
	}
	w = req(app, "GET", "/api/v1/status", "", true)
	if strings.Contains(w.Body.String(), "private") {
		t.Fatal("diagnostic leaked")
	}
}
func TestLoginLogoutAndExpiry(t *testing.T) {
	app := setup(t, nil)
	w := req(app, "POST", "/api/v1/auth/login", `{"password":"test-only-password"}`, false)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].Domain != "" {
		t.Fatal("cookie")
	}
	var data struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		}
	}
	json.Unmarshal(w.Body.Bytes(), &data)
	r := httptest.NewRequest("POST", "http://127.0.0.1:19080/api/v1/auth/logout", strings.NewReader("{}"))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", app.Config.PublicURL)
	r.Header.Set("X-CSRF-Token", data.Data.CSRF)
	r.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	r.Method = "GET"
	r.URL.Path = "/api/v1/auth/session"
	w = httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("logout did not revoke")
	}
	req(app, "GET", "/api/v1/status", "", true)
	app.mu.Lock()
	v := app.sessions["test"]
	v.Last = time.Now().Add(-31 * time.Minute)
	app.mu.Unlock()
	w = req(app, "GET", "/api/v1/auth/session", "", true)
	if w.Code != 401 {
		t.Fatal("idle expiry")
	}
	select {
	case <-v.Done:
	default:
		t.Fatal("streams not notified")
	}
}
func TestSummaryClassification(t *testing.T) {
	now := time.Now()
	base := managerStatus{ObservedAt: now.UTC().Format(time.RFC3339), Core: coreStatus{StateQueryOK: true, ServiceState: "active", Running: true, ServiceActive: true, ControllerHealthy: true}, TUN: tunStatus{ObservationOK: true}}
	cases := []struct {
		name, want string
		change     func(*managerStatus)
	}{{"healthy", "healthy", func(*managerStatus) {}}, {"query failed", "unknown", func(s *managerStatus) { s.Core.StateQueryOK = false }}, {"stopped", "stopped", func(s *managerStatus) { s.Core = coreStatus{StateQueryOK: true, ServiceState: "inactive"} }}, {"failed", "degraded", func(s *managerStatus) { s.Core.ServiceState = "failed" }}, {"tun mismatch", "degraded", func(s *managerStatus) { s.TUN.Configured = true }}, {"controller", "degraded", func(s *managerStatus) { s.Core.ControllerHealthy = false; s.Core.Running = false }}, {"stale", "unknown", func(s *managerStatus) { s.ObservedAt = now.Add(-time.Minute).Format(time.RFC3339) }}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := base
			tc.change(&s)
			if got := summaryStatus(s, true, false, now)["state"]; got != tc.want {
				t.Fatalf("%v", got)
			}
		})
	}
}
func TestSSEBlocksAndLogoutClosesUpstream(t *testing.T) {
	closed := make(chan struct{})
	app := setup(t, func(w http.ResponseWriter, r *http.Request) {
		defer close(closed)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `{"type":"info","payload":"hel`)
		w.(http.Flusher).Flush()
		fmt.Fprint(w, "lo"+`"}`+"\ndata: {\"type\":\"warning\",\"payload\":\"world\"}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	req(app, "GET", "/api/v1/auth/session", "", true)
	web := httptest.NewServer(app)
	defer web.Close()
	r, _ := http.NewRequest("GET", web.URL+"/api/v1/logs/stream", nil)
	r.Host = "127.0.0.1:19080"
	r.AddCookie(&http.Cookie{Name: cookieName, Value: "test"})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	res, e := http.DefaultClient.Do(r.WithContext(ctx))
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	b := make([]byte, 1024)
	n, e := res.Body.Read(b)
	if e != nil || !strings.Contains(string(b[:n]), "event: log") {
		t.Fatalf("%s %v", b[:n], e)
	}
	app.revoke("test")
	_, _ = io.ReadAll(res.Body)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("upstream survived logout")
	}
}
func TestWriteBusyDoesNotQueue(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	app := setup(t, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		fmt.Fprint(w, `{"success":true,"data":{}}`)
	})
	done := make(chan struct{})
	go func() { defer close(done); req(app, "POST", "/api/v1/core/start", "{}", true) }()
	<-entered
	w := req(app, "POST", "/api/v1/core/stop", "{}", true)
	if w.Code != 409 {
		t.Fatal("write queued")
	}
	close(release)
	<-done
}

func TestProductionSecureCookie(t *testing.T) {
	app := setup(t, nil)
	app.Config.TestMode = false
	app.Config.PublicURL = "https://127.0.0.1:19080"
	r := httptest.NewRequest("POST", app.Config.PublicURL+"/api/v1/auth/login", strings.NewReader(`{"password":"test-only-password"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", app.Config.PublicURL)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 || !w.Result().Cookies()[0].Secure {
		t.Fatalf("%d: secure cookie missing", w.Code)
	}
}
func TestQuietStreamSurvivesMinute(t *testing.T) {
	if os.Getenv("MIHOMO_WEB_LONG_STREAM_TEST") != "1" {
		t.Skip("显式运行一分钟静默流验收")
	}
	app := setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	req(app, "GET", "/api/v1/auth/session", "", true)
	web := httptest.NewServer(app)
	defer web.Close()
	r, _ := http.NewRequest("GET", web.URL+"/api/v1/logs/stream", nil)
	r.Host = "127.0.0.1:19080"
	r.AddCookie(&http.Cookie{Name: cookieName, Value: "test"})
	ctx, cancel := context.WithTimeout(context.Background(), 66*time.Second)
	defer cancel()
	res, e := http.DefaultClient.Do(r.WithContext(ctx))
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if strings.Count(string(body), ": heartbeat") < 4 {
		t.Fatalf("静默流未保持一分钟：%q", body)
	}
}
