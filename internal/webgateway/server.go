// Package webgateway exposes an authenticated, explicitly mapped browser API.
package webgateway

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"mihomotui/internal/webconfig"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const cookieName = "mihomo_web_session"

type session struct {
	CSRF          string
	Created, Last time.Time
	Done          chan struct{}
}
type Server struct {
	Config               webconfig.Config
	Static               string
	client, stream       *http.Client
	mu                   sync.Mutex
	sessions             map[string]*session
	loginStart           time.Time
	loginAttempts        int
	passwordBusy         chan struct{}
	writeBusy, delayBusy chan struct{}
	closed               chan struct{}
	closeOnce            sync.Once
}

func New(c webconfig.Config, static string) (*Server, error) {
	if e := c.Validate(); e != nil {
		return nil, e
	}
	tr := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", c.ManagerSocket)
	}, ResponseHeaderTimeout: 120 * time.Second, MaxIdleConns: 8, IdleConnTimeout: 30 * time.Second}
	st := tr.Clone()
	st.ResponseHeaderTimeout = 5 * time.Second
	return &Server{Config: c, Static: static, client: &http.Client{Transport: tr, CheckRedirect: noRedirect}, stream: &http.Client{Transport: st, CheckRedirect: noRedirect}, sessions: map[string]*session{}, passwordBusy: make(chan struct{}, 1), writeBusy: make(chan struct{}, 1), delayBusy: make(chan struct{}, 1), closed: make(chan struct{})}, nil
}
func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		close(s.closed)
		s.mu.Lock()
		for id, v := range s.sessions {
			close(v.Done)
			delete(s.sessions, id)
		}
		s.mu.Unlock()
		s.client.CloseIdleConnections()
		s.stream.CloseIdleConnections()
	})
}
func reply(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func success(w http.ResponseWriter, status int, data any) {
	reply(w, status, map[string]any{"data": data, "request_id": webconfig.RandomToken()[:16]})
}
func failure(w http.ResponseWriter, status int, code, message string) {
	reply(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "retryable": false}, "request_id": webconfig.RandomToken()[:16]})
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	if r.URL.Path == "/healthz" {
		if r.Method != "GET" {
			failure(w, 405, "INVALID_INPUT", "方法不允许")
			return
		}
		reply(w, 200, map[string]any{"app": "mihomo-web", "pid": os.Getpid()})
		return
	}
	if r.URL.Path == "/api/v1/summary" {
		s.summary(w, r)
		return
	}
	// Summary credentials are never accepted as browser credentials, even alongside a cookie.
	if r.Header.Get("Authorization") != "" {
		failure(w, 403, "FORBIDDEN", "此令牌不能访问该接口")
		return
	}
	public, _ := url.Parse(s.Config.PublicURL)
	if !strings.EqualFold(r.Host, public.Host) {
		failure(w, 403, "FORBIDDEN", "访问地址不匹配")
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		s.static(w, r)
		return
	}
	if r.Method != "GET" {
		if r.Header.Get("Origin") != strings.TrimRight(s.Config.PublicURL, "/") {
			failure(w, 403, "FORBIDDEN", "请求来源无效")
			return
		}
		if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
			failure(w, 400, "INVALID_INPUT", "需要 JSON 请求")
			return
		}
	}
	if r.ContentLength > 64<<10 {
		failure(w, 400, "INVALID_INPUT", "请求过大")
		return
	}
	if r.URL.Path == "/api/v1/auth/login" {
		s.login(w, r)
		return
	}
	id, v := s.getSession(r)
	if v == nil {
		failure(w, 401, "UNAUTHORIZED", "请重新登录")
		return
	}
	if r.Method != "GET" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(v.CSRF)) != 1 {
		failure(w, 403, "FORBIDDEN", "会话校验失败")
		return
	}
	switch r.URL.Path {
	case "/api/v1/auth/session":
		if r.Method != "GET" {
			failure(w, 405, "INVALID_INPUT", "方法不允许")
			return
		}
		success(w, 200, map[string]any{"user": "管理员", "csrf_token": v.CSRF, "permissions": []string{"read", "operate"}})
	case "/api/v1/auth/logout":
		if r.Method != "POST" {
			failure(w, 405, "INVALID_INPUT", "方法不允许")
			return
		}
		var empty struct{}
		if !decodeBody(w, r, &empty) {
			return
		}
		s.revoke(id)
		http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", MaxAge: -1, HttpOnly: true, Secure: !s.Config.TestMode, SameSite: http.SameSiteLaxMode})
		success(w, 200, map[string]bool{"logged_out": true})
	case "/api/v1/auth/refresh":
		if r.Method != "POST" {
			failure(w, 405, "INVALID_INPUT", "方法不允许")
			return
		}
		var empty struct{}
		if !decodeBody(w, r, &empty) {
			return
		}
		s.mu.Lock()
		if cur := s.sessions[id]; cur != nil {
			cur.Last = time.Now()
		}
		s.mu.Unlock()
		success(w, 200, map[string]bool{"refreshed": true})
	case "/api/v1/capabilities":
		if r.Method != "GET" {
			failure(w, 405, "INVALID_INPUT", "方法不允许")
			return
		}
		success(w, 200, map[string]any{"schema_version": 1, "actions": []string{"status", "core", "tun", "proxy-port", "profiles", "proxy-groups", "proxy-delay", "rules", "logs", "logging"}, "web_lifecycle": false, "delay_concurrency": 1})
	case "/api/v1/logs/stream":
		if r.Method != "GET" {
			failure(w, 405, "INVALID_INPUT", "方法不允许")
			return
		}
		s.logs(w, r, id, v)
	default:
		s.forward(w, r)
	}
}
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		failure(w, 405, "INVALID_INPUT", "方法不允许")
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" || name == "overview" || name == "profiles" || name == "proxies" || name == "rules" || name == "logs" || name == "login" {
		name = "index.html"
	}
	if !filepath.IsLocal(name) || strings.Contains(name, "\\") || strings.HasPrefix(name, ".") {
		http.NotFound(w, r)
		return
	}
	target := filepath.Join(s.Static, name)
	info, e := os.Stat(target)
	if e != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	// Release directories are root-owned; no runtime upload path is served.
	http.ServeFile(w, r, target)
}
func (s *Server) getSession(r *http.Request) (string, *session) {
	c, e := r.Cookie(cookieName)
	if e != nil {
		return "", nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.sessions[c.Value]
	if v == nil {
		return "", nil
	}
	now := time.Now()
	if now.Sub(v.Created) >= 12*time.Hour || now.Sub(v.Last) >= 30*time.Minute {
		close(v.Done)
		delete(s.sessions, c.Value)
		return "", nil
	}
	copy := *v
	return c.Value, &copy
}
func (s *Server) revoke(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v := s.sessions[id]; v != nil {
		close(v.Done)
		delete(s.sessions, id)
	}
}
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(dst); e != nil {
		failure(w, 400, "INVALID_INPUT", "请求字段无效或过大")
		return false
	}
	if d.Decode(new(any)) != io.EOF {
		failure(w, 400, "INVALID_INPUT", "请求必须是单个 JSON 对象")
		return false
	}
	return true
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		failure(w, 405, "INVALID_INPUT", "方法不允许")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	s.mu.Lock()
	now := time.Now()
	if now.Sub(s.loginStart) > time.Minute {
		s.loginStart = now
		s.loginAttempts = 0
	}
	s.loginAttempts++
	limited := s.loginAttempts > 10
	s.mu.Unlock()
	if limited {
		w.Header().Set("Retry-After", "60")
		failure(w, 429, "RATE_LIMITED", "登录尝试过多，请稍后再试")
		return
	}
	select {
	case s.passwordBusy <- struct{}{}:
		defer func() { <-s.passwordBusy }()
	default:
		failure(w, 429, "RATE_LIMITED", "请稍后重试")
		return
	}
	if !webconfig.CheckPassword(s.Config.PasswordHash, body.Password) {
		failure(w, 401, "UNAUTHORIZED", "密码不正确")
		return
	}
	id := webconfig.RandomToken()
	v := &session{CSRF: webconfig.RandomToken(), Created: now, Last: now, Done: make(chan struct{})}
	s.mu.Lock()
	for key, old := range s.sessions {
		if now.Sub(old.Created) >= 12*time.Hour || now.Sub(old.Last) >= 30*time.Minute {
			close(old.Done)
			delete(s.sessions, key)
		}
	}
	if len(s.sessions) >= 16 {
		s.mu.Unlock()
		failure(w, 429, "RATE_LIMITED", "会话数量已达上限，请退出旧会话")
		return
	}
	s.sessions[id] = v
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: id, Path: "/", MaxAge: 12 * 3600, HttpOnly: true, Secure: !s.Config.TestMode, SameSite: http.SameSiteLaxMode})
	success(w, 200, map[string]string{"csrf_token": v.CSRF, "user": "管理员"})
}
func (s *Server) upstream(ctx context.Context, method, path string, body io.Reader) (json.RawMessage, int, error) {
	req, e := http.NewRequestWithContext(ctx, method, "http://manager"+path, body)
	if e != nil {
		return nil, 502, e
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, e := s.client.Do(req)
	if e != nil {
		return nil, 502, e
	}
	defer res.Body.Close()
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	d := json.NewDecoder(io.LimitReader(res.Body, 16<<20))
	if e = d.Decode(&envelope); e != nil {
		return nil, 502, e
	}
	if res.StatusCode >= 400 || !envelope.Success {
		return nil, res.StatusCode, fmt.Errorf("上游操作失败")
	}
	return envelope.Data, res.StatusCode, nil
}
