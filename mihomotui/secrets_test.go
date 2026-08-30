package mihomotui

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFlushSeparatesSecretsFromShareableConfig(t *testing.T) {
	useTestConfigDir(t)
	cfg := defaultConfig()
	cfg.Mihomo.Secret = "api-secret-must-not-leak"
	cfg.Subscriptions = []SubscriptionMeta{{
		ID: "sub-1", Name: "demo", URL: "https://example.invalid/sub?token=subscription-secret", SourceType: SubscriptionSourceURL,
	}}
	cfg.RuleProviderSubscriptions = []RuleProviderSubscription{{
		Name: "private-rules", URL: "https://example.invalid/rules?key=rule-secret",
	}}
	if err := cfg.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	publicData, err := os.ReadFile(configFilePath())
	if err != nil {
		t.Fatal(err)
	}
	publicText := string(publicData)
	for _, leaked := range []string{"api-secret-must-not-leak", "subscription-secret", "rule-secret"} {
		if strings.Contains(publicText, leaked) {
			t.Fatalf("shareable config leaked %q:\n%s", leaked, publicText)
		}
	}
	for _, ref := range []string{"secret://subscription/sub-1", "secret://rule-provider/private-rules"} {
		if !strings.Contains(publicText, ref) {
			t.Fatalf("shareable config missing reference %q:\n%s", ref, publicText)
		}
	}

	secretData, err := os.ReadFile(secretsFilePath())
	if err != nil {
		t.Fatal(err)
	}
	secretText := string(secretData)
	for _, expected := range []string{"api-secret-must-not-leak", "subscription-secret", "rule-secret"} {
		if !strings.Contains(secretText, expected) {
			t.Fatalf("private config missing %q", expected)
		}
	}
	for path, wantMode := range map[string]os.FileMode{
		secretsDirPath():  0700,
		secretsFilePath(): 0600,
		configFilePath():  0600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != wantMode {
			t.Fatalf("%s mode = %04o, want %04o", path, got, wantMode)
		}
	}

	loaded := LoadConfig()
	if loaded.Mihomo.Secret != cfg.Mihomo.Secret || loaded.Subscriptions[0].URL != cfg.Subscriptions[0].URL || loaded.RuleProviderSubscriptions[0].URL != cfg.RuleProviderSubscriptions[0].URL {
		t.Fatalf("LoadConfig() did not merge private values: %+v", loaded)
	}
}

func TestLoadConfigMigratesLegacyInlineSecrets(t *testing.T) {
	useTestConfigDir(t)
	legacy := defaultConfig()
	legacy.Mihomo.Secret = "legacy-api-secret"
	legacy.Subscriptions = []SubscriptionMeta{{
		ID: "legacy-sub", Name: "legacy", URL: "https://example.invalid/sub?token=legacy-token", SourceType: SubscriptionSourceURL,
	}}
	data, err := yaml.Marshal(&legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFilePath(), data, 0600); err != nil {
		t.Fatal(err)
	}

	loaded := LoadConfig()
	if loaded.Mihomo.Secret != "legacy-api-secret" || loaded.Subscriptions[0].URL != legacy.Subscriptions[0].URL {
		t.Fatalf("legacy values were not preserved in memory: %+v", loaded)
	}
	publicData, err := os.ReadFile(configFilePath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(publicData), "legacy-api-secret") || strings.Contains(string(publicData), "legacy-token") {
		t.Fatalf("legacy inline values were not removed from public config:\n%s", publicData)
	}
	if _, err := os.Stat(secretsFilePath()); err != nil {
		t.Fatalf("private config was not created: %v", err)
	}
}
