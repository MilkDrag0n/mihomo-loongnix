package webgateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestExternalModeSessionAndCSRF(t *testing.T) {
	var writes atomic.Int32
	app := setup(t, func(w http.ResponseWriter, r *http.Request) {
		writes.Add(1)
		fmt.Fprint(w, "{\"success\":true,\"data\":{\"enabled\":true}}")
	})
	app.Config.AuthMode = "external"
	app.Config.PasswordHash = ""
	if err := app.Config.Validate(); err != nil {
		t.Fatal(err)
	}
	if w := req(app, "GET", "/api/v1/auth/mode", "", false); w.Code != 200 || !strings.Contains(w.Body.String(), "\"external\"") {
		t.Fatal("external authentication mode not advertised")
	}
	if w := req(app, "POST", "/api/v1/auth/login", "{}", false); w.Code != 405 {
		t.Fatal("password login should be disabled")
	}
	if w := req(app, "PUT", "/api/v1/logging", "{\"enabled\":true}", false); w.Code != 401 {
		t.Fatal("write was allowed without a session")
	}
	for _, variant := range []string{"cross-site", "bearer"} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:19080/api/v1/auth/session", nil)
		if variant == "cross-site" {
			r.Header.Set("Sec-Fetch-Site", "cross-site")
		} else {
			r.Header.Set("Authorization", "Bearer "+app.Config.SummaryToken)
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != 403 || len(w.Result().Cookies()) != 0 {
			t.Fatalf("%s unexpectedly acquired a session", variant)
		}
	}
	opened := req(app, "GET", "/api/v1/auth/session", "", false)
	if opened.Code != 200 || len(opened.Result().Cookies()) != 1 {
		t.Fatal("external session did not open")
	}
	var result struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(opened.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	for _, valid := range []bool{false, true} {
		r := httptest.NewRequest("PUT", "http://127.0.0.1:19080/api/v1/logging", strings.NewReader("{\"enabled\":true}"))
		r.Header.Set("Origin", app.Config.PublicURL)
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(opened.Result().Cookies()[0])
		if valid {
			r.Header.Set("X-CSRF-Token", result.Data.CSRF)
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		want := 403
		if valid {
			want = 200
		}
		if w.Code != want {
			t.Fatalf("valid CSRF=%v: got %d, want %d", valid, w.Code, want)
		}
	}
	if writes.Load() != 1 {
		t.Fatalf("manager received %d writes", writes.Load())
	}
}
