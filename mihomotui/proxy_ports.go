package mihomotui

// localHTTPProxyPort returns a local listener that accepts HTTP proxy traffic.
// A mixed listener is preferred because it preserves the single-port layout
// commonly used by existing Clash/Mihomo installations.
func (c *Config) localHTTPProxyPort() int {
	if c.Mihomo.MixedPort > 0 {
		return c.Mihomo.MixedPort
	}
	return c.Mihomo.HTTPPort
}

// localSOCKSProxyPort returns a local listener that accepts SOCKS traffic.
func (c *Config) localSOCKSProxyPort() int {
	if c.Mihomo.MixedPort > 0 {
		return c.Mihomo.MixedPort
	}
	return c.Mihomo.SOCKS5Port
}
