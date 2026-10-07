package mihomotui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuntimeMetricsOnlyReturnsAggregateCounters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/memory" {
			w.Write([]byte(`{"inuse":8000000}`))
			return
		}
		if r.Method != "GET" || r.URL.Path != "/connections" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"downloadTotal":4200,"uploadTotal":1200,"memory":8000000,"connections":[{"metadata":{"host":"private.invalid"},"id":"private-id"}]}`))
	}))
	defer server.Close()
	m := runtimeMetrics(NewMihomoAPI(server.URL, ""))
	if m == nil || *m.Connections != 1 || *m.DownloadTotal != 4200 || *m.MemoryBytes != 8000000 {
		t.Fatalf("invalid aggregates: %+v", m)
	}
	b, _ := json.Marshal(m)
	if strings.Contains(string(b), "private") || strings.Contains(string(b), "metadata") {
		t.Fatal("connection detail leaked")
	}
}
func TestRuntimeMetricsMissingInvalidAndZeroAreDistinct(t *testing.T) {
	for _, tc := range []struct {
		body  string
		known bool
	}{{`{}`, false}, {`{"downloadTotal":-1}`, false}, {`{"downloadTotal":"bad"}`, false}, {`{"connections":null,"downloadTotal":0,"uploadTotal":0}`, true}, {`{"memory":0}`, false}} {
		t.Run(tc.body, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(tc.body)) }))
			defer s.Close()
			m := runtimeMetrics(NewMihomoAPI(s.URL, ""))
			if (m != nil) != tc.known {
				t.Fatalf("known=%v, result=%+v", tc.known, m)
			}
			if m != nil && tc.body == `{"memory":0}` && m.DownloadTotal != nil {
				t.Fatal("missing total changed to zero")
			}
		})
	}
}
