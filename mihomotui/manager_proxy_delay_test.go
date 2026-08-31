package mihomotui

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type activeCoreController struct{}

func (activeCoreController) Start(context.Context) error   { return nil }
func (activeCoreController) Stop(context.Context) error    { return nil }
func (activeCoreController) Restart(context.Context) error { return nil }
func (activeCoreController) State(context.Context) (coreServiceState, error) {
	return coreServiceState{Active: true, PID: 42}, nil
}

func TestManagerProxyDelayReturnsMeasuredKernelValue(t *testing.T) {
	useTestConfigDir(t)
	var delayRequestSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version":
			_, _ = io.WriteString(w, `{"version":"1.19.27"}`)
		case "/configs":
			_, _ = io.WriteString(w, `{"tun":{"enable":false}}`)
		case "/proxies":
			_, _ = io.WriteString(w, `{"proxies":{"Auto":{"name":"Auto","type":"Selector","now":"node-1","all":["node-1"]},"node-1":{"name":"node-1","type":"Shadowsocks"}}}`)
		case "/proxies/node-1/delay":
			delayRequestSeen = true
			if got := r.URL.Query().Get("timeout"); got != "5000" {
				t.Errorf("timeout = %q, want 5000", got)
			}
			_, _ = io.WriteString(w, `{"delay":73}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := *GlobalConfig()
	cfg.Mihomo.ExternalController = strings.TrimPrefix(server.URL, "http://")
	cfg.MihomoRunningVersion = "1.19.27"
	cfg.Mihomo.TestURL = "http://cp.cloudflare.com/generate_204"
	SetGlobalConfig(cfg)
	d := &Daemon{core: activeCoreController{}}
	body, _ := json.Marshal(ProxyDelayTestRequest{Group: "Auto", Name: "node-1"})
	recorder := httptest.NewRecorder()
	d.handleManagerProxyDelay(recorder, httptest.NewRequest(http.MethodPost, "/v1/proxy-delay", bytes.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response APIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	result, err := unmarshalData[ProxyDelayResponse](&response)
	if err != nil {
		t.Fatal(err)
	}
	if !delayRequestSeen || result.Name != "node-1" || result.Delay != 73 {
		t.Fatalf("delay request seen=%v result=%+v", delayRequestSeen, result)
	}
}
