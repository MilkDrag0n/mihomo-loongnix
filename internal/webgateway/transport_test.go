package webgateway

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestQuietUpstreamMayDelayHeadersPastFiveSeconds(t *testing.T) {
	for _, scenario := range []string{"first_log", "logout_while_waiting"} {
		t.Run(scenario, func(t *testing.T) {
			entered, closed := make(chan struct{}), make(chan struct{})
			app := setup(t, func(w http.ResponseWriter, r *http.Request) {
				close(entered)
				defer close(closed)
				// The real core's HTTP log endpoint sends nothing before a matching event.
				select {
				case <-r.Context().Done():
					return
				case <-time.After(6 * time.Second):
				}
				fmt.Fprintln(w, "{\"type\":\"warning\",\"payload\":\"delayed demo log\"}")
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			})
			req(app, "GET", "/api/v1/auth/session", "", true)
			web := httptest.NewServer(app)
			defer web.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			r, _ := http.NewRequestWithContext(ctx, "GET", web.URL+"/api/v1/logs/stream", nil)
			r.Host = "127.0.0.1:19080"
			r.AddCookie(&http.Cookie{Name: cookieName, Value: "test"})
			client := &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: time.Second}}
			defer client.CloseIdleConnections()
			response, err := client.Do(r)
			if err != nil {
				t.Fatalf("downstream headers must not await core logs: %v", err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("stream status = %d", response.StatusCode)
			}
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("upstream subscription did not start")
			}
			if scenario == "first_log" {
				scanner := bufio.NewScanner(response.Body)
				found := false
				for scanner.Scan() {
					if strings.Contains(scanner.Text(), "delayed demo log") {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("first log lost after quiet interval: %v", scanner.Err())
				}
			}
			app.revoke("test")
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("logout did not cancel the upstream request")
			}
		})
	}
}

func TestHTTPSReverseProxyLoginWriteAndLogout(t *testing.T) {
	var writes atomic.Int32
	app := setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/v1/logging" {
			writes.Add(1)
		}
		fmt.Fprint(w, "{\"success\":true,\"data\":{\"enabled\":true}}")
	})
	origin := httptest.NewServer(app)
	defer origin.Close()
	target, _ := url.Parse(origin.URL)
	edge := httptest.NewUnstartedServer(httputil.NewSingleHostReverseProxy(target))
	app.Config.PublicURL = "https://example.com"
	app.Config.TestMode = false
	if err := app.Config.Validate(); err != nil {
		t.Fatal(err)
	}
	edge.StartTLS()
	defer edge.Close()
	// Client trusts only the test server's generated certificate; TLS verification stays on.
	client := edge.Client()
	transport := client.Transport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, edge.Listener.Addr().String())
	}
	client.Transport = transport
	defer client.CloseIdleConnections()
	client.Timeout = 5 * time.Second
	client.Jar, _ = cookiejar.New(nil)
	csrf := ""
	call := func(method, path, body string) *http.Response {
		t.Helper()
		r, err := http.NewRequest(method, app.Config.PublicURL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", app.Config.PublicURL)
		r.Header.Set("X-CSRF-Token", csrf)
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	login := call("POST", "/api/v1/auth/login", "{\"password\":\"test-only-password\"}")
	if login.StatusCode != http.StatusOK {
		login.Body.Close()
		t.Fatalf("HTTPS login status = %d", login.StatusCode)
	}
	cookies := login.Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Fatal("production cookie flags missing")
	}
	var session struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(login.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	login.Body.Close()
	httpURL, _ := url.Parse(strings.Replace(app.Config.PublicURL, "https://", "http://", 1))
	if len(client.Jar.Cookies(httpURL)) != 0 {
		t.Fatal("secure session would be sent over plain HTTP")
	}
	denied := call("PUT", "/api/v1/logging", "{\"enabled\":true}")
	denied.Body.Close()
	if denied.StatusCode != http.StatusForbidden || writes.Load() != 0 {
		t.Fatal("write without CSRF reached manager")
	}
	csrf = session.Data.CSRF
	applied := call("PUT", "/api/v1/logging", "{\"enabled\":true}")
	applied.Body.Close()
	if applied.StatusCode != http.StatusOK || writes.Load() != 1 {
		t.Fatal("authenticated HTTPS write failed")
	}
	logout := call("POST", "/api/v1/auth/logout", "{}")
	logout.Body.Close()
	if logout.StatusCode != http.StatusOK {
		t.Fatal("HTTPS logout failed")
	}
	after := call("GET", "/api/v1/auth/session", "")
	after.Body.Close()
	if after.StatusCode != http.StatusUnauthorized {
		t.Fatal("session survived HTTPS logout")
	}
}
