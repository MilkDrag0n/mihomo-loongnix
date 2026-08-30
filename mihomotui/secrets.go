package mihomotui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	secretsDirName  = "secrets"
	secretsFileName = "runtime.yaml"
	secretsVersion  = 1
)

// runtimeSecrets contains values that must never be written to the shareable
// application config. The whole directory is private to the daemon/user and is
// intentionally covered by the repository's .gitignore as a second boundary.
type runtimeSecrets struct {
	Version             int               `yaml:"version"`
	MihomoAPISecret     string            `yaml:"mihomo_api_secret,omitempty"`
	SubscriptionSources map[string]string `yaml:"subscription_sources,omitempty"`
	RuleProviderURLs    map[string]string `yaml:"rule_provider_urls,omitempty"`
}

func secretsDirPath() string {
	return filepath.Join(GetConfigDir(), secretsDirName)
}

func secretsFilePath() string {
	return filepath.Join(secretsDirPath(), secretsFileName)
}

func subscriptionSecretRef(id string) string {
	return "secret://subscription/" + id
}

func ruleProviderSecretRef(name string) string {
	return "secret://rule-provider/" + SanitizeFileName(name)
}

func isSecretRef(value string) bool {
	return strings.HasPrefix(value, "secret://")
}

func shouldSeparateSubscriptionSource(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "手动配置" && !isSecretRef(value)
}

func secretsFromConfig(cfg *Config) runtimeSecrets {
	secrets := runtimeSecrets{
		Version:             secretsVersion,
		MihomoAPISecret:     cfg.Mihomo.Secret,
		SubscriptionSources: make(map[string]string),
		RuleProviderURLs:    make(map[string]string),
	}
	for _, sub := range cfg.Subscriptions {
		if sub.ID != "" && shouldSeparateSubscriptionSource(sub.URL) {
			secrets.SubscriptionSources[sub.ID] = sub.URL
		}
	}
	for _, provider := range cfg.RuleProviderSubscriptions {
		if provider.Name != "" && strings.TrimSpace(provider.URL) != "" && !isSecretRef(provider.URL) {
			secrets.RuleProviderURLs[provider.Name] = provider.URL
		}
	}
	return secrets
}

// publicConfigForDisk returns the version safe to copy, review, or commit. The
// full in-memory Config still contains credentials for daemon operations.
func publicConfigForDisk(cfg *Config) Config {
	public := cfg.Clone()
	public.Mihomo.Secret = ""
	for i := range public.Subscriptions {
		if shouldSeparateSubscriptionSource(public.Subscriptions[i].URL) {
			public.Subscriptions[i].URL = subscriptionSecretRef(public.Subscriptions[i].ID)
		}
	}
	for i := range public.RuleProviderSubscriptions {
		if strings.TrimSpace(public.RuleProviderSubscriptions[i].URL) != "" && !isSecretRef(public.RuleProviderSubscriptions[i].URL) {
			public.RuleProviderSubscriptions[i].URL = ruleProviderSecretRef(public.RuleProviderSubscriptions[i].Name)
		}
	}
	return public
}

func configContainsInlineSecrets(cfg *Config) bool {
	if strings.TrimSpace(cfg.Mihomo.Secret) != "" {
		return true
	}
	for _, sub := range cfg.Subscriptions {
		if shouldSeparateSubscriptionSource(sub.URL) {
			return true
		}
	}
	for _, provider := range cfg.RuleProviderSubscriptions {
		if strings.TrimSpace(provider.URL) != "" && !isSecretRef(provider.URL) {
			return true
		}
	}
	return false
}

func loadRuntimeSecrets(cfg *Config) error {
	path := secretsFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取私密配置失败: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("收紧私密配置权限失败: %w", err)
	}
	var secrets runtimeSecrets
	if err := yaml.Unmarshal(data, &secrets); err != nil {
		return fmt.Errorf("解析私密配置失败: %w", err)
	}
	if secrets.MihomoAPISecret != "" {
		cfg.Mihomo.Secret = secrets.MihomoAPISecret
	}
	for i := range cfg.Subscriptions {
		if source := secrets.SubscriptionSources[cfg.Subscriptions[i].ID]; source != "" {
			cfg.Subscriptions[i].URL = source
		}
	}
	for i := range cfg.RuleProviderSubscriptions {
		if rawURL := secrets.RuleProviderURLs[cfg.RuleProviderSubscriptions[i].Name]; rawURL != "" {
			cfg.RuleProviderSubscriptions[i].URL = rawURL
		}
	}
	return nil
}

func writeRuntimeSecrets(cfg *Config) error {
	dir := secretsDirPath()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建私密配置目录失败: %w", err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("收紧私密配置目录权限失败: %w", err)
	}
	data, err := yaml.Marshal(secretsFromConfig(cfg))
	if err != nil {
		return fmt.Errorf("序列化私密配置失败: %w", err)
	}
	if err := writePrivateFileAtomically(secretsFilePath(), data); err != nil {
		return fmt.Errorf("保存私密配置失败: %w", err)
	}
	return nil
}

func writePrivateFileAtomically(path string, data []byte) error {
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Chmod(path, 0600)
}
