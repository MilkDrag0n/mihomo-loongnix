package mihomotui

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

type inactivePortCoreController struct{}

func (inactivePortCoreController) Start(context.Context) error   { return nil }
func (inactivePortCoreController) Stop(context.Context) error    { return nil }
func (inactivePortCoreController) Restart(context.Context) error { return nil }
func (inactivePortCoreController) State(context.Context) (coreServiceState, error) {
	return coreServiceState{}, nil
}

func TestManagerStatusReportsRuntimePortAndCurrentNode(t *testing.T) {
	useTestConfigDir(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version":
			_, _ = io.WriteString(w, `{"version":"1.19.28"}`)
		case "/configs":
			_, _ = io.WriteString(w, `{"mixed-port":17890,"tun":{"enable":false}}`)
		case "/proxies/Auto":
			_, _ = io.WriteString(w, `{"name":"Auto","type":"Selector","now":"node-1","all":["node-1"]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := *GlobalConfig()
	cfg.Mihomo.ExternalController = strings.TrimPrefix(server.URL, "http://")
	cfg.DefaultProxyGroup = "Auto"
	SetGlobalConfig(cfg)

	status, err := (&Daemon{core: activeCoreController{}}).managerStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Core.Running || status.ProxyPort != 17890 || status.CurrentGroup != "Auto" || status.CurrentNode != "node-1" {
		t.Fatalf("status = %+v", status)
	}
}

func TestManagerProxyPortPersistsWhenCoreStopped(t *testing.T) {
	useTestConfigDir(t)
	cfg := *GlobalConfig()
	cfg.Subscriptions = []SubscriptionMeta{{ID: "demo", Name: "demo", URL: "https://example.com/sub"}}
	cfg.ActiveSubscription = 0
	SetGlobalConfig(cfg)

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	body, _ := json.Marshal(ProxyPortSetRequest{Port: port})
	recorder := httptest.NewRecorder()
	(&Daemon{core: inactivePortCoreController{}}).handleManagerProxyPort(recorder, httptest.NewRequest(http.MethodPut, "/v1/proxy-port", bytes.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := GlobalConfig().Mihomo.MixedPort; got != port {
		t.Fatalf("persisted mixed port=%d, want %d", got, port)
	}
	var response APIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	status, err := unmarshalData[ManagerStatus](&response)
	if err != nil || status.ProxyPort != port {
		t.Fatalf("response status=%+v err=%v", status, err)
	}
}

func TestManagerProxyPortRejectsControllerAndOccupiedPorts(t *testing.T) {
	useTestConfigDir(t)
	d := &Daemon{core: inactivePortCoreController{}}
	for _, port := range []int{0, 65536, 9090} {
		body, _ := json.Marshal(ProxyPortSetRequest{Port: port})
		recorder := httptest.NewRecorder()
		d.handleManagerProxyPort(recorder, httptest.NewRequest(http.MethodPut, "/v1/proxy-port", bytes.NewReader(body)))
		if recorder.Code < 400 {
			t.Fatalf("port %d accepted: status=%d", port, recorder.Code)
		}
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	body, _ := json.Marshal(ProxyPortSetRequest{Port: port})
	recorder := httptest.NewRecorder()
	d.handleManagerProxyPort(recorder, httptest.NewRequest(http.MethodPut, "/v1/proxy-port", bytes.NewReader(body)))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("occupied port status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestManagerProxyPortReloadsAndReadsBackRunningCore(t *testing.T) {
	useTestConfigDir(t)
	runtimePort := 7890
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			_, _ = io.WriteString(w, `{"version":"1.19.28"}`)
		case r.URL.Path == "/configs" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"mixed-port":`+strconv.Itoa(runtimePort)+`,"tun":{"enable":false}}`)
		case r.URL.Path == "/configs" && r.Method == http.MethodPut:
			runtimePort = GlobalConfig().Mihomo.MixedPort
			_, _ = io.WriteString(w, `{}`)
		case r.URL.Path == "/proxies/Auto":
			_, _ = io.WriteString(w, `{"name":"Auto","type":"Selector","now":"node-1"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cfg := *GlobalConfig()
	cfg.Mihomo.ExternalController = strings.TrimPrefix(server.URL, "http://")
	cfg.Subscriptions = []SubscriptionMeta{{ID: "demo", Name: "demo", URL: "https://example.com/sub"}}
	cfg.ActiveSubscription = 0
	SetGlobalConfig(cfg)

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	body, _ := json.Marshal(ProxyPortSetRequest{Port: port})
	recorder := httptest.NewRecorder()
	(&Daemon{core: activeCoreController{}}).handleManagerProxyPort(recorder, httptest.NewRequest(http.MethodPut, "/v1/proxy-port", bytes.NewReader(body)))
	if recorder.Code != http.StatusOK || runtimePort != port || GlobalConfig().Mihomo.MixedPort != port {
		t.Fatalf("status=%d runtime=%d persisted=%d body=%s", recorder.Code, runtimePort, GlobalConfig().Mihomo.MixedPort, recorder.Body.String())
	}
}

func TestManagerProxyPortRollsBackWhenReloadFails(t *testing.T) {
	useTestConfigDir(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			_, _ = io.WriteString(w, `{"version":"1.19.28"}`)
		case r.URL.Path == "/configs" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"mixed-port":7890,"tun":{"enable":false}}`)
		case r.URL.Path == "/configs" && r.Method == http.MethodPut:
			http.Error(w, "reload failed", http.StatusInternalServerError)
		case r.URL.Path == "/proxies/Auto":
			_, _ = io.WriteString(w, `{"name":"Auto","type":"Selector","now":"node-1"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cfg := *GlobalConfig()
	cfg.Mihomo.ExternalController = strings.TrimPrefix(server.URL, "http://")
	cfg.Subscriptions = []SubscriptionMeta{{ID: "demo", Name: "demo", URL: "https://example.com/sub"}}
	cfg.ActiveSubscription = 0
	SetGlobalConfig(cfg)

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	body, _ := json.Marshal(ProxyPortSetRequest{Port: port})
	recorder := httptest.NewRecorder()
	(&Daemon{core: activeCoreController{}}).handleManagerProxyPort(recorder, httptest.NewRequest(http.MethodPut, "/v1/proxy-port", bytes.NewReader(body)))
	if recorder.Code < 400 || GlobalConfig().Mihomo.MixedPort != 7890 {
		t.Fatalf("status=%d persisted=%d body=%s", recorder.Code, GlobalConfig().Mihomo.MixedPort, recorder.Body.String())
	}
}
