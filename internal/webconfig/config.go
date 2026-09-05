// Package webconfig describes the optional Web service, independently of its UI.
package webconfig

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const ProductionPath = "/etc/mihomo-web/config.json"
const ProductionSocket = "/run/mihomo-tui/daemon.sock"

type Config struct {
	Listen        string `json:"listen"`
	PublicURL     string `json:"public_url"`
	ManagerSocket string `json:"manager_socket"`
	PasswordHash  string `json:"password_hash"`
	SummaryToken  string `json:"summary_token"`
	ShowNode      bool   `json:"show_node"`
	TestMode      bool   `json:"test_mode"`
}

func Load(path string) (Config, error) {
	var c Config
	f, e := os.Open(path)
	if e != nil {
		return c, e
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 65537))
	d.DisallowUnknownFields()
	if e = d.Decode(&c); e != nil {
		return c, fmt.Errorf("Web 配置格式无效")
	}
	if d.Decode(new(any)) != io.EOF {
		return c, fmt.Errorf("Web 配置包含多余内容")
	}
	return c, c.Validate()
}
func (c Config) Validate() error {
	host, port, e := net.SplitHostPort(c.Listen)
	if e != nil {
		return fmt.Errorf("Web 监听地址无效")
	}
	ip := net.ParseIP(host)
	n, e := strconv.Atoi(port)
	if ip == nil || !ip.IsLoopback() || e != nil || n < 1 || n > 65535 {
		return fmt.Errorf("Web 必须监听明确的本机回环地址和有效端口")
	}
	u, e := url.Parse(c.PublicURL)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.Opaque != "" {
		return fmt.Errorf("公开入口必须是无路径及凭据的地址")
	}
	if u.Scheme != "https" && !(c.TestMode && u.Scheme == "http" && net.ParseIP(u.Hostname()).IsLoopback()) {
		return fmt.Errorf("正式公开入口必须使用 HTTPS")
	}
	if !filepath.IsAbs(c.ManagerSocket) || filepath.Clean(c.ManagerSocket) != c.ManagerSocket {
		return fmt.Errorf("必须显式配置绝对 socket 路径")
	}
	if c.TestMode && (c.ManagerSocket == ProductionSocket || strings.HasPrefix(c.ManagerSocket, "/var/lib/mihomo-tui/")) {
		return fmt.Errorf("测试禁止使用正式 socket")
	}
	if len(c.SummaryToken) < 32 {
		return fmt.Errorf("摘要令牌至少 32 字符")
	}
	if _, _, e = decodeHash(c.PasswordHash); e != nil {
		return e
	}
	return nil
}
func RandomToken() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func HashPassword(password string) (string, error) {
	if len(password) < 12 || len(password) > 1024 {
		return "", fmt.Errorf("密码须为 12—1024 字节")
	}
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		return "", e
	}
	key, e := pbkdf2.Key(sha256.New, password, salt, 600000, 32)
	if e != nil {
		return "", e
	}
	return "pbkdf2-sha256$600000$" + hex.EncodeToString(salt) + "$" + hex.EncodeToString(key), nil
}
func decodeHash(hash string) ([]byte, []byte, error) {
	p := strings.Split(hash, "$")
	if len(p) != 4 || p[0] != "pbkdf2-sha256" || p[1] != "600000" {
		return nil, nil, fmt.Errorf("密码哈希无效")
	}
	salt, e := hex.DecodeString(p[2])
	if e != nil || len(salt) != 16 {
		return nil, nil, fmt.Errorf("密码盐无效")
	}
	key, e := hex.DecodeString(p[3])
	if e != nil || len(key) != 32 {
		return nil, nil, fmt.Errorf("密码哈希无效")
	}
	return salt, key, nil
}
func CheckPassword(hash, password string) bool {
	salt, want, e := decodeHash(hash)
	if e != nil || len(password) > 1024 {
		return false
	}
	got, e := pbkdf2.Key(sha256.New, password, salt, 600000, 32)
	return e == nil && subtle.ConstantTimeCompare(got, want) == 1
}
