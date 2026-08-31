package mihomotui

import "testing"

func TestLocalProxyPortsPreferMixedListener(t *testing.T) {
	cfg := defaultConfig()
	if got := cfg.localHTTPProxyPort(); got != cfg.Mihomo.MixedPort {
		t.Fatalf("HTTP proxy port = %d, want mixed port %d", got, cfg.Mihomo.MixedPort)
	}
	if got := cfg.localSOCKSProxyPort(); got != cfg.Mihomo.MixedPort {
		t.Fatalf("SOCKS proxy port = %d, want mixed port %d", got, cfg.Mihomo.MixedPort)
	}

	cfg.Mihomo.HTTPPort = 0
	cfg.Mihomo.SOCKS5Port = 0
	cfg.Mihomo.MixedPort = 7890
	if got := cfg.localHTTPProxyPort(); got != 7890 {
		t.Fatalf("mixed-only HTTP proxy port = %d, want 7890", got)
	}
	if got := cfg.localSOCKSProxyPort(); got != 7890 {
		t.Fatalf("mixed-only SOCKS proxy port = %d, want 7890", got)
	}
}

func TestLocalProxyPortsFallBackToDedicatedListeners(t *testing.T) {
	cfg := defaultConfig()
	cfg.Mihomo.MixedPort = 0
	cfg.Mihomo.HTTPPort = 8080
	cfg.Mihomo.SOCKS5Port = 1080
	if got := cfg.localHTTPProxyPort(); got != 8080 {
		t.Fatalf("HTTP proxy port = %d, want 8080", got)
	}
	if got := cfg.localSOCKSProxyPort(); got != 1080 {
		t.Fatalf("SOCKS proxy port = %d, want 1080", got)
	}
}
