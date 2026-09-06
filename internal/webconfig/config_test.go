package webconfig

import (
	"strings"
	"testing"
)

func TestConfigurationBoundaries(t *testing.T) {
	hash, e := HashPassword("test-only-password")
	if e != nil {
		t.Fatal(e)
	}
	c := Config{Listen: "127.0.0.1:19080", PublicURL: "http://127.0.0.1:19080", ManagerSocket: "/tmp/test.sock", PasswordHash: hash, SummaryToken: strings.Repeat("x", 32), TestMode: true}
	if c.Validate() != nil {
		t.Fatal(c.Validate())
	}
	if !CheckPassword(hash, "test-only-password") || CheckPassword(hash, "bad") {
		t.Fatal("password verification")
	}
	for _, change := range []func(*Config){func(c *Config) { c.ManagerSocket = ProductionSocket }, func(c *Config) { c.Listen = "0.0.0.0:9080" }, func(c *Config) { c.PublicURL = "https://user:pass@example.com" }, func(c *Config) { c.TestMode = false }, func(c *Config) { c.PasswordHash = "invalid" }} {
		x := c
		change(&x)
		if x.Validate() == nil {
			t.Fatal("invalid config accepted")
		}
	}
}

func TestExternalAuthenticationIsExplicitAndHasNoPassword(t *testing.T) {
	c := Config{Listen: "127.0.0.1:9080", PublicURL: "https://example.com", ManagerSocket: ProductionSocket, SummaryToken: strings.Repeat("x", 32)}
	if c.Validate() == nil {
		t.Fatal("an omitted auth mode must still require a password")
	}
	c.AuthMode = "external"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.PasswordHash = "unused-password"
	if c.Validate() == nil {
		t.Fatal("external configuration should not retain a password")
	}
	c.PasswordHash = ""
	c.AuthMode = "typo"
	if c.Validate() == nil {
		t.Fatal("unknown auth mode accepted")
	}
}
