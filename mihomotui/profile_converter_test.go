package mihomotui

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNormalizeProfileContentYAMLAndBase64URIs(t *testing.T) {
	yamlInput := []byte("proxies:\n  - name: demo\n    type: socks5\n    server: 127.0.0.1\n    port: 1080\n")
	got, kind, err := normalizeProfileContent(yamlInput)
	if err != nil || kind != "yaml" || !strings.Contains(string(got), "demo") {
		t.Fatalf("yaml: kind=%s err=%v output=%s", kind, err, got)
	}
	uriList := "ss://YWVzLTEyOC1nY206cGFzcw@127.0.0.1:8388#SS%20node\n" + "vless://00000000-0000-0000-0000-000000000001@example.com:443?security=tls&type=ws&path=%2Fws#VL"
	encoded := base64.StdEncoding.EncodeToString([]byte(uriList))
	got, kind, err = normalizeProfileContent([]byte(encoded))
	if err != nil || kind != "uri-list" {
		t.Fatalf("uri: kind=%s err=%v", kind, err)
	}
	var root map[string]any
	if err := yaml.Unmarshal(got, &root); err != nil {
		t.Fatal(err)
	}
	proxies, ok := root["proxies"].([]any)
	if !ok || len(proxies) != 2 {
		t.Fatalf("proxies=%#v", root["proxies"])
	}
}

func TestNormalizeProfileRejectsPartialURIList(t *testing.T) {
	_, _, err := normalizeProfileContent([]byte("ss://YWVzLTEyOC1nY206cGFzcw@127.0.0.1:8388#ok\nnot-a-node"))
	if err == nil || !strings.Contains(err.Error(), "未保存") {
		t.Fatalf("err=%v", err)
	}
}

func TestNormalizeProfileRejectsStructurallyInvalidProxyYAML(t *testing.T) {
	for _, input := range []string{
		"proxies:\n  - not-a-proxy\n",
		"proxies:\n  - name: missing-type\n    server: 127.0.0.1\n",
	} {
		if _, _, err := normalizeProfileContent([]byte(input)); err == nil {
			t.Fatalf("accepted invalid provider: %s", input)
		}
	}
}

func TestProfileListNeverExposesSubscriptionPathOrToken(t *testing.T) {
	useTestConfigDir(t)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("proxies:\n  - name: demo\n    type: socks5\n    server: 127.0.0.1\n    port: 1080\n"))
	}))
	defer provider.Close()
	rawURL := provider.URL + "/private/token-value?auth=query-secret#fragment-secret"
	body := strings.NewReader(`{"name":"demo","url":"` + rawURL + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/profiles", body)
	recorder := httptest.NewRecorder()
	(&Daemon{}).handleProfiles(recorder, req)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, secret := range []string{"private", "token-value", "query-secret", "fragment-secret"} {
		if strings.Contains(recorder.Body.String(), secret) {
			t.Fatalf("profile response exposed %q: %s", secret, recorder.Body.String())
		}
	}
	var envelope struct {
		Data ProfileSummary `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Source != provider.URL {
		t.Fatalf("source=%q want host-only %q", envelope.Data.Source, provider.URL)
	}
	cfg := GlobalConfig()
	if len(cfg.Subscriptions) != 1 {
		t.Fatalf("subscriptions=%d", len(cfg.Subscriptions))
	}
	info, err := os.Stat(cfg.Subscriptions[0].CacheFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("cache permissions=%#o", info.Mode().Perm())
	}
}

func TestParseSystemdShow(t *testing.T) {
	state := parseSystemdShow("MainPID=42\nActiveState=active\n")
	if !state.Active || state.PID != 42 {
		t.Fatalf("state=%+v", state)
	}
}

func TestManagerRuntimeNetworkOnlyOverridesInExplicitShadowMode(t *testing.T) {
	useTestConfigDir(t)
	if _, err := UpdateGlobalConfig(func(c *Config) error { c.Mihomo.MixedPort = 7888; return nil }); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MIHOMO_TUI_SHADOW", "")
	t.Setenv("MIHOMO_TUI_SHADOW_MIXED_PORT", "12345")
	mixed, controller, err := managerRuntimeNetwork()
	if err != nil || mixed != 7888 || controller != "127.0.0.1:9090" {
		t.Fatalf("production network=%d,%q err=%v", mixed, controller, err)
	}
	t.Setenv("MIHOMO_TUI_SHADOW", "1")
	t.Setenv("MIHOMO_TUI_SHADOW_MIXED_PORT", "17891")
	t.Setenv("MIHOMO_TUI_SHADOW_CONTROLLER_PORT", "19091")
	mixed, controller, err = managerRuntimeNetwork()
	if err != nil || mixed != 17891 || controller != "127.0.0.1:19091" {
		t.Fatalf("shadow network=%d,%q err=%v", mixed, controller, err)
	}
	t.Setenv("MIHOMO_TUI_SHADOW_CONTROLLER_PORT", "17891")
	if _, _, err := managerRuntimeNetwork(); err == nil {
		t.Fatal("accepted identical shadow ports")
	}
}

func TestSocketDirectoryRejectsSharedRoots(t *testing.T) {
	for _, path := range []string{"/", "/tmp", "/run", "/var/run"} {
		if !socketDirectoryIsUnsafe(path) {
			t.Fatalf("shared socket directory accepted: %s", path)
		}
	}
	if socketDirectoryIsUnsafe("/tmp/mihomo-tui-shadow") {
		t.Fatal("dedicated socket directory rejected")
	}
}

func TestTUNCleanupSkippedForKnownCleanState(t *testing.T) {
	if tunCleanupNeeded(TUNRuntimeStatus{}) {
		t.Fatal("clean TUN state requested privileged cleanup")
	}
	if !tunCleanupNeeded(TUNRuntimeStatus{RuntimeEnabled: true}) {
		t.Fatal("runtime TUN state did not request cleanup")
	}
}
