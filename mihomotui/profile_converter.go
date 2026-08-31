package mihomotui

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// normalizeProfileContent converts supported URI subscriptions into the
// proxy-provider YAML shape consumed by mihomo. Existing Clash/Mihomo YAML is
// preserved after structural validation.
func normalizeProfileContent(content []byte) ([]byte, string, error) {
	text := strings.TrimSpace(string(content))
	if text == "" {
		return nil, "", fmt.Errorf("配置内容为空")
	}
	if normalized, ok := validProviderYAML([]byte(text)); ok {
		return normalized, "yaml", nil
	}
	if decoded, ok := decodeProfileBase64(text); ok {
		if normalized, yamlOK := validProviderYAML(decoded); yamlOK {
			return normalized, "base64-yaml", nil
		}
		text = strings.TrimSpace(string(decoded))
	}
	proxies, err := parseProxyURIs(text)
	if err != nil {
		return nil, "", err
	}
	data, err := yaml.Marshal(map[string]any{"proxies": proxies})
	if err != nil {
		return nil, "", fmt.Errorf("生成 provider YAML 失败: %w", err)
	}
	return data, "uri-list", nil
}

func validProviderYAML(content []byte) ([]byte, bool) {
	var root map[string]any
	if yaml.Unmarshal(content, &root) != nil {
		return nil, false
	}
	proxies, ok := root["proxies"].([]any)
	if !ok || len(proxies) == 0 {
		return nil, false
	}
	for _, raw := range proxies {
		proxy, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(stringValue(proxy["name"])) == "" || strings.TrimSpace(stringValue(proxy["type"])) == "" {
			return nil, false
		}
	}
	data, err := yaml.Marshal(root)
	return data, err == nil
}

func decodeProfileBase64(text string) ([]byte, bool) {
	compact := strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, text)
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := encoding.DecodeString(compact); err == nil && len(decoded) > 0 && isRecognizedSubscriptionContent(strings.TrimSpace(string(decoded))) {
			return decoded, true
		}
	}
	return nil, false
}

func parseProxyURIs(text string) ([]any, error) {
	var proxies []any
	var problems []string
	for lineNo, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		proxy, err := parseProxyURI(line, len(proxies)+1)
		if err != nil {
			problems = append(problems, fmt.Sprintf("第 %d 行: %v", lineNo+1, err))
			continue
		}
		proxies = append(proxies, proxy)
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("URI 配置解析失败（未保存）: %s", strings.Join(problems, "；"))
	}
	if len(proxies) == 0 {
		return nil, fmt.Errorf("没有可用代理节点")
	}
	return proxies, nil
}

func parseProxyURI(raw string, index int) (map[string]any, error) {
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "vmess://") {
		return parseVMessURI(raw, index)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("链接格式无效")
	}
	name := uriNodeName(u, index)
	server, port, err := uriEndpoint(u)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	base := map[string]any{"name": name, "server": server, "port": port}
	switch strings.ToLower(u.Scheme) {
	case "ss":
		method, password, err := ssCredentials(u)
		if err != nil {
			return nil, err
		}
		base["type"], base["cipher"], base["password"], base["udp"] = "ss", method, password, true
		if plugin := q.Get("plugin"); plugin != "" {
			base["plugin"] = strings.Split(plugin, ";")[0]
		}
	case "vless":
		base["type"], base["uuid"], base["udp"] = "vless", u.User.Username(), true
		applyV2RayTransport(base, q)
		if flow := q.Get("flow"); flow != "" {
			base["flow"] = flow
		}
	case "trojan":
		base["type"], base["password"], base["udp"] = "trojan", u.User.Username(), true
		applyV2RayTransport(base, q)
	case "hysteria2", "hy2":
		base["type"], base["password"] = "hysteria2", u.User.Username()
		if sni := first(q.Get("sni"), q.Get("peer")); sni != "" {
			base["sni"] = sni
		}
		if insecure(q) {
			base["skip-cert-verify"] = true
		}
		if obfs := q.Get("obfs"); obfs != "" {
			base["obfs"] = obfs
		}
		if value := first(q.Get("obfs-password"), q.Get("obfsParam")); value != "" {
			base["obfs-password"] = value
		}
	case "hysteria":
		base["type"], base["auth-str"] = "hysteria", u.User.Username()
		if sni := first(q.Get("sni"), q.Get("peer")); sni != "" {
			base["sni"] = sni
		}
		if insecure(q) {
			base["skip-cert-verify"] = true
		}
		if up := first(q.Get("upmbps"), q.Get("up")); up != "" {
			base["up"] = up
		}
		if down := first(q.Get("downmbps"), q.Get("down")); down != "" {
			base["down"] = down
		}
	case "tuic":
		base["type"], base["uuid"] = "tuic", u.User.Username()
		if password, ok := u.User.Password(); ok {
			base["password"] = password
		}
		if sni := q.Get("sni"); sni != "" {
			base["sni"] = sni
		}
		if insecure(q) {
			base["skip-cert-verify"] = true
		}
	case "socks5", "socks":
		base["type"], base["udp"] = "socks5", true
		if u.User != nil {
			base["username"] = u.User.Username()
			if password, ok := u.User.Password(); ok {
				base["password"] = password
			}
		}
	default:
		return nil, fmt.Errorf("暂不支持 %s:// 节点", u.Scheme)
	}
	return base, nil
}

func parseVMessURI(raw string, index int) (map[string]any, error) {
	payload := strings.TrimPrefix(raw, "vmess://")
	var decoded []byte
	var ok bool
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, _ = encoding.DecodeString(payload); len(decoded) > 0 {
			ok = true
			break
		}
	}
	if !ok {
		return nil, fmt.Errorf("vmess 内容不是有效 Base64")
	}
	var v map[string]any
	if err := json.Unmarshal(decoded, &v); err != nil {
		return nil, fmt.Errorf("vmess JSON 无效")
	}
	server := stringValue(v["add"])
	port, err := strconv.Atoi(stringValue(v["port"]))
	if err != nil || server == "" {
		return nil, fmt.Errorf("vmess 缺少服务器或端口")
	}
	name := stringValue(v["ps"])
	if name == "" {
		name = fmt.Sprintf("vmess-%d", index)
	}
	proxy := map[string]any{"name": name, "type": "vmess", "server": server, "port": port, "uuid": stringValue(v["id"]), "alterId": intValue(v["aid"]), "cipher": first(stringValue(v["scy"]), "auto"), "udp": true}
	if strings.EqualFold(stringValue(v["tls"]), "tls") {
		proxy["tls"] = true
	}
	if sni := first(stringValue(v["sni"]), stringValue(v["host"])); sni != "" {
		proxy["servername"] = sni
	}
	network := stringValue(v["net"])
	if network != "" && network != "tcp" {
		proxy["network"] = network
	}
	if network == "ws" {
		headers := map[string]any{}
		if host := stringValue(v["host"]); host != "" {
			headers["Host"] = host
		}
		proxy["ws-opts"] = map[string]any{"path": first(stringValue(v["path"]), "/"), "headers": headers}
	}
	return proxy, nil
}

func uriEndpoint(u *url.URL) (string, int, error) {
	server := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil || server == "" || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("服务器或端口无效")
	}
	return server, port, nil
}

func uriNodeName(u *url.URL, index int) string {
	if name, err := url.QueryUnescape(u.Fragment); err == nil && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return fmt.Sprintf("%s-%d", strings.ToLower(u.Scheme), index)
}

func ssCredentials(u *url.URL) (string, string, error) {
	if u.User == nil {
		return "", "", fmt.Errorf("ss 缺少加密方式和密码")
	}
	if password, ok := u.User.Password(); ok {
		return u.User.Username(), password, nil
	}
	raw := u.User.Username()
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := encoding.DecodeString(raw); err == nil {
			if method, password, ok := strings.Cut(string(decoded), ":"); ok {
				return method, password, nil
			}
		}
	}
	return "", "", fmt.Errorf("ss 凭据无效")
}

func applyV2RayTransport(proxy map[string]any, q url.Values) {
	security := strings.ToLower(first(q.Get("security"), q.Get("tls")))
	if security == "tls" || security == "reality" || security == "1" || security == "true" {
		proxy["tls"] = true
	}
	if security == "reality" {
		proxy["reality-opts"] = map[string]any{"public-key": q.Get("pbk"), "short-id": q.Get("sid")}
	}
	if sni := first(q.Get("sni"), q.Get("servername")); sni != "" {
		proxy["servername"] = sni
	}
	if insecure(q) {
		proxy["skip-cert-verify"] = true
	}
	network := first(q.Get("type"), q.Get("network"))
	if network != "" && network != "tcp" {
		proxy["network"] = network
	}
	if network == "ws" {
		headers := map[string]any{}
		if host := q.Get("host"); host != "" {
			headers["Host"] = host
		}
		path, _ := url.QueryUnescape(first(q.Get("path"), "/"))
		proxy["ws-opts"] = map[string]any{"path": path, "headers": headers}
	}
	if fingerprint := first(q.Get("fp"), q.Get("fingerprint")); fingerprint != "" {
		proxy["client-fingerprint"] = fingerprint
	}
}

func insecure(q url.Values) bool {
	value := strings.ToLower(first(q.Get("allowInsecure"), q.Get("insecure")))
	return value == "1" || value == "true"
}
func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
func intValue(value any) int { n, _ := strconv.Atoi(stringValue(value)); return n }
