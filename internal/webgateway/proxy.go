package webgateway

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type coreStatus struct {
	ServiceActive     bool   `json:"service_active"`
	ControllerHealthy bool   `json:"controller_healthy"`
	Running           bool   `json:"running"`
	StateQueryOK      bool   `json:"state_query_ok"`
	ServiceState      string `json:"service_state"`
	PID               int    `json:"pid"`
}
type tunStatus struct {
	Configured       bool `json:"configured"`
	RuntimeEnabled   bool `json:"runtime_enabled"`
	InterfacePresent bool `json:"interface_present"`
	Enabled          bool `json:"enabled"`
	ObservationOK    bool `json:"observation_ok"`
}
type profile struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Source  string `json:"source"`
	Updated string `json:"updated_at,omitempty"`
	Active  bool   `json:"active"`
}
type managerStatus struct {
	Core          coreStatus `json:"core"`
	TUN           tunStatus  `json:"tun"`
	ActiveProfile *profile   `json:"active_profile,omitempty"`
	ProxyPort     int        `json:"proxy_port"`
	CurrentGroup  string     `json:"current_group,omitempty"`
	CurrentNode   string     `json:"current_node,omitempty"`
	ObservedAt    string     `json:"observed_at"`
}

func (s *Server) forward(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1")
	target := "/v1" + path
	valid := false
	kind := ""
	body := []byte(nil)
	switch path {
	case "/status":
		valid = r.Method == "GET"
		kind = "status"
	case "/core/start", "/core/stop":
		valid = r.Method == "POST"
		kind = "status"
	case "/tun", "/logging":
		valid = r.Method == "PUT"
		if valid {
			var b struct {
				Enabled *bool `json:"enabled"`
			}
			if !decodeBody(w, r, &b) {
				return
			}
			if b.Enabled == nil {
				failure(w, 400, "INVALID_INPUT", "必须显式提供 enabled")
				return
			}
			body, _ = json.Marshal(b)
		}
		if path == "/tun" {
			kind = "status"
		} else {
			kind = "logging"
		}
	case "/proxy-port":
		valid = r.Method == "PUT"
		kind = "status"
		if valid {
			var b struct {
				Port *int `json:"port"`
			}
			if !decodeBody(w, r, &b) {
				return
			}
			if b.Port == nil || *b.Port < 1 || *b.Port > 65535 {
				failure(w, 400, "INVALID_INPUT", "端口必须为 1—65535 的整数")
				return
			}
			body, _ = json.Marshal(b)
		}
	case "/profiles":
		valid = r.Method == "GET" || r.Method == "POST"
		kind = "profiles"
		if r.Method == "POST" {
			var b struct {
				Name string `json:"name"`
				URL  string `json:"url"`
			}
			if !decodeBody(w, r, &b) {
				return
			}
			u, e := url.Parse(b.URL)
			if e != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") {
				failure(w, 400, "INVALID_INPUT", "订阅地址需要 HTTP 或 HTTPS")
				return
			}
			body, _ = json.Marshal(b)
			kind = "profile"
		}
	case "/proxy-groups", "/rules":
		valid = r.Method == "GET"
	case "/proxy-delay":
		valid = r.Method == "POST"
		if valid {
			var b struct {
				Group string `json:"group"`
				Name  string `json:"name"`
			}
			if !decodeBody(w, r, &b) {
				return
			}
			if strings.TrimSpace(b.Name) == "" || strings.TrimSpace(b.Group) == "" {
				failure(w, 400, "INVALID_INPUT", "需要完整组名和节点名称")
				return
			}
			body, _ = json.Marshal(b)
		}
	case "/logging/status":
		valid = r.Method == "GET"
		kind = "logging"
	default:
		if strings.HasPrefix(path, "/profiles/") {
			parts := strings.Split(strings.TrimPrefix(path, "/profiles/"), "/")
			if len(parts) > 2 || parts[0] == "" || strings.ContainsAny(parts[0], "%\\") {
				failure(w, 404, "NOT_FOUND", "配置路径无效")
				return
			}
			target = "/v1/profiles/" + url.PathEscape(parts[0])
			kind = "profile"
			if len(parts) == 2 && (parts[1] == "activate" || parts[1] == "update") {
				valid = r.Method == "POST"
				target += "/" + parts[1]
			} else if len(parts) == 1 {
				valid = r.Method == "PATCH" || r.Method == "DELETE"
				if r.Method == "PATCH" {
					var b struct {
						Name string `json:"name"`
					}
					if !decodeBody(w, r, &b) {
						return
					}
					if strings.TrimSpace(b.Name) == "" {
						failure(w, 400, "INVALID_INPUT", "名称不能为空")
						return
					}
					body, _ = json.Marshal(b)
				} else {
					kind = "profiles"
				}
			}
		} else if strings.HasPrefix(path, "/proxy-groups/") {
			name := strings.TrimPrefix(path, "/proxy-groups/")
			valid = r.Method == "PUT" && name != ""
			target = "/v1/proxy-groups/" + url.PathEscape(name)
			if valid {
				var b struct {
					Name string `json:"name"`
				}
				if !decodeBody(w, r, &b) {
					return
				}
				if strings.TrimSpace(b.Name) == "" {
					failure(w, 400, "INVALID_INPUT", "节点名称不能为空")
					return
				}
				body, _ = json.Marshal(b)
			}
		}
	}
	if !valid {
		failure(w, 404, "NOT_FOUND", "接口或方法不存在")
		return
	}
	if r.Method != "GET" && body == nil {
		var empty struct{}
		if !decodeBody(w, r, &empty) {
			return
		}
	}
	if r.URL.RawQuery != "" {
		failure(w, 400, "INVALID_INPUT", "此接口不接受查询参数")
		return
	}
	timeout := 5 * time.Second
	if r.Method != "GET" {
		timeout = 120 * time.Second
		lock := s.writeBusy
		if path == "/proxy-delay" {
			lock = s.delayBusy
		}
		select {
		case lock <- struct{}{}:
			defer func() { <-lock }()
		default:
			failure(w, 409, "BUSY", "已有操作进行中，请稍后刷新状态")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	data, status, e := s.upstream(ctx, r.Method, target, bytes.NewReader(body))
	if e != nil {
		if r.Method != "GET" && (ctx.Err() != nil || status >= 500) {
			failure(w, 504, "RESULT_UNKNOWN", "结果待确认，请刷新实际状态，不要重复提交")
			return
		}
		if ctx.Err() != nil {
			code, message := "UPSTREAM_TIMEOUT", "查询超时"
			if r.Method != "GET" {
				code, message = "RESULT_UNKNOWN", "结果待确认，请刷新实际状态，不要重复提交"
			}
			failure(w, 504, code, message)
			return
		}
		codes := map[int]string{400: "INVALID_INPUT", 403: "FORBIDDEN", 404: "NOT_FOUND", 409: "CONFLICT", 504: "RESULT_UNKNOWN"}
		code := codes[status]
		if code == "" {
			status, code = 502, "UPSTREAM_UNAVAILABLE"
		}
		failure(w, status, code, "管理器未完成操作，请刷新状态检查结果")
		return
	}
	var result any
	switch kind {
	case "status":
		var v managerStatus
		e = json.Unmarshal(data, &v)
		result = v
	case "profile":
		var v profile
		e = json.Unmarshal(data, &v)
		result = v
	case "profiles":
		var v []profile
		e = json.Unmarshal(data, &v)
		if v == nil {
			v = []profile{}
		}
		result = v
	case "logging":
		var v map[string]any
		e = json.Unmarshal(data, &v)
		result = map[string]any{"enabled": v["enabled"], "current_file_bytes": v["current_file_bytes"], "total_bytes": v["total_bytes"], "max_file_bytes": v["max_file_bytes"], "max_backups": v["max_backups"], "has_error": v["last_error"] != nil && v["last_error"] != ""}
	default:
		e = json.Unmarshal(data, &result)
	}
	if e != nil {
		failure(w, 502, "UPSTREAM_UNAVAILABLE", "管理器返回格式无效")
		return
	}
	success(w, status, result)
}
func (s *Server) summary(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		failure(w, 403, "FORBIDDEN", "摘要只允许读取")
		return
	}
	want := "Bearer " + s.Config.SummaryToken
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(want)) != 1 {
		failure(w, 401, "UNAUTHORIZED", "摘要令牌无效")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	data, _, e := s.upstream(ctx, "GET", "/v1/status", nil)
	var status managerStatus
	if e == nil {
		e = json.Unmarshal(data, &status)
	}
	result := summaryStatus(status, e == nil, s.Config.ShowNode, time.Now())
	reply(w, 200, result)
}
func summaryStatus(s managerStatus, readOK, showNode bool, now time.Time) map[string]any {
	state, label := "unknown", "状态未知"
	stamp, e := time.Parse(time.RFC3339, s.ObservedAt)
	stale := e != nil || now.Sub(stamp) > 30*time.Second || stamp.After(now.Add(5*time.Second))
	if readOK && !stale && s.Core.StateQueryOK {
		switch {
		case s.Core.ServiceState == "inactive":
			state, label = "stopped", "代理已停止"
		case s.Core.ServiceState == "failed" || s.Core.ServiceState == "activating" || s.Core.ServiceState == "deactivating":
			state, label = "degraded", "代理服务异常或正在切换"
		case s.Core.ServiceActive && !s.Core.ControllerHealthy:
			state, label = "degraded", "控制接口不可用"
		case s.Core.Running && s.TUN.ObservationOK && s.TUN.Configured != s.TUN.Enabled:
			state, label = "degraded", "TUN 状态与配置不一致"
		case s.Core.Running && s.TUN.ObservationOK:
			state, label = "healthy", "运行正常"
		}
	}
	if !readOK || stale {
		state, label = "unknown", "数据不可用"
		stale = true
	}
	if e == nil && now.Sub(stamp) > 30*time.Second {
		label = "数据过期"
	}
	node := "未选择"
	if s.CurrentNode != "" {
		node = "已选择"
		if showNode {
			node = s.CurrentNode
		}
	}
	tun := "未知"
	if readOK && s.TUN.ObservationOK {
		tun = "关闭"
		if s.TUN.Configured {
			tun = "待生效"
		}
		if s.TUN.Enabled {
			tun = "已开启"
		}
	}
	var observed any
	if e == nil {
		observed = s.ObservedAt
	}
	return map[string]any{"schema_version": 1, "app": "mihomo", "state": state, "state_label": label, "observed_at": observed, "stale": stale, "node_label": node, "tun_label": tun}
}
