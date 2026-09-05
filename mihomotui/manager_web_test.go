package mihomotui

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
)

type fakeWeb struct {
	running bool
	calls   int
}

func (f *fakeWeb) Status(context.Context) (WebStatus, error) {
	s := WebStatus{Installed: true, Configured: true, State: "stopped", Running: f.running, ServiceActive: f.running, Healthy: f.running}
	if f.running {
		s.State = "running"
	}
	return s, nil
}
func (f *fakeWeb) Action(ctx context.Context, a string) (WebStatus, error) {
	f.calls++
	if a != "start" && a != "stop" {
		return WebStatus{}, fmt.Errorf("bad action")
	}
	f.running = a == "start"
	return f.Status(ctx)
}
func TestWebControlIsIndependentAndShadowFailsClosed(t *testing.T) {
	t.Setenv("MIHOMO_TUI_SHADOW", "1")
	d := &Daemon{}
	w := httptest.NewRecorder()
	d.router().ServeHTTP(w, httptest.NewRequest("POST", "/v1/web/start", nil))
	if w.Code != 503 {
		t.Fatal("test fell through to systemd")
	}
	f := &fakeWeb{}
	d.web = f
	d.actionMu.Lock()
	defer d.actionMu.Unlock()
	for _, a := range []string{"start", "stop"} {
		w = httptest.NewRecorder()
		d.router().ServeHTTP(w, httptest.NewRequest("POST", "/v1/web/"+a, nil))
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	if f.running || f.calls != 2 {
		t.Fatal("web lifecycle")
	}
	for _, tc := range []struct {
		m, p string
		want bool
	}{{"GET", "/v1/web/status", true}, {"POST", "/v1/web/start", false}, {"POST", "/v1/web/stop", false}} {
		if isIPCReadOnlyRequest(httptest.NewRequest(tc.m, tc.p, nil)) != tc.want {
			t.Fatal(tc)
		}
	}
}
