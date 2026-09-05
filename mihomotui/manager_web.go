package mihomotui

import (
	"context"
	"encoding/json"
	"fmt"
	"mihomotui/internal/webconfig"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const webUnit = "mihomo-web.service"

type WebStatus struct {
	Installed     bool   `json:"installed"`
	Configured    bool   `json:"configured"`
	State         string `json:"state"`
	ServiceActive bool   `json:"service_active"`
	Healthy       bool   `json:"healthy"`
	Running       bool   `json:"running"`
	PublicURL     string `json:"public_url"`
	ObservedAt    string `json:"observed_at"`
	ErrorCode     string `json:"error_code,omitempty"`
	Message       string `json:"message,omitempty"`
}
type webController interface {
	Status(context.Context) (WebStatus, error)
	Action(context.Context, string) (WebStatus, error)
}
type systemWebController struct{}

func (d *Daemon) webControl() webController {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.web != nil {
		return d.web
	}
	// 普通测试和嵌入运行不允许退回生产 systemd。
	if os.Getenv("MIHOMO_TUI_SHADOW") == "1" || os.Geteuid() != 0 {
		return nil
	}
	return systemWebController{}
}
func webCommand(ctx context.Context, args ...string) (string, error) {
	b, e := exec.CommandContext(ctx, "systemctl", args...).Output()
	return strings.TrimSpace(string(b)), e
}
func (systemWebController) Status(ctx context.Context) (WebStatus, error) {
	s := WebStatus{State: "unknown", ObservedAt: time.Now().UTC().Format(time.RFC3339)}
	b, e := webCommand(ctx, "show", webUnit, "--property=LoadState,ActiveState,MainPID")
	if e != nil {
		return s, fmt.Errorf("无法读取 Web 服务状态")
	}
	props := map[string]string{}
	for _, l := range strings.Split(b, "\n") {
		k, v, ok := strings.Cut(l, "=")
		if ok {
			props[k] = v
		}
	}
	if props["LoadState"] == "not-found" {
		s.State = "not_installed"
		return s, nil
	}
	s.ServiceActive = props["ActiveState"] == "active"
	root, e := filepath.EvalSymlinks("/opt/mihomo-web/current")
	if e == nil {
		_, be := os.Stat(filepath.Join(root, "mihomo-web-linux-loong64"))
		_, se := os.Stat(filepath.Join(root, "static/index.html"))
		s.Installed = be == nil && se == nil
	}
	if !s.Installed {
		s.State = "not_installed"
		s.ErrorCode = "WEB_NOT_INSTALLED"
		return s, nil
	}
	c, e := webconfig.Load(webconfig.ProductionPath)
	s.Configured = e == nil && !c.TestMode
	if s.Configured {
		s.PublicURL = c.PublicURL
	} else {
		s.ErrorCode = "WEB_NOT_CONFIGURED"
		s.Message = "Web 配置尚未完成"
	}
	switch props["ActiveState"] {
	case "inactive":
		s.State = "stopped"
	case "activating":
		s.State = "starting"
	case "deactivating":
		s.State = "stopping"
	case "failed":
		s.State = "failed"
	case "active":
		s.State = "failed"
	default:
		return s, fmt.Errorf("Web 服务状态未知")
	}
	if s.ServiceActive && s.Configured {
		client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}}
		req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+c.Listen+"/healthz", nil)
		resp, e := client.Do(req)
		if e == nil {
			defer resp.Body.Close()
			var health struct {
				App string `json:"app"`
				PID int    `json:"pid"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&health)
			pid, _ := strconv.Atoi(props["MainPID"])
			s.Healthy = resp.StatusCode == 200 && health.App == "mihomo-web" && pid > 0 && health.PID == pid
		}
		client.CloseIdleConnections()
		s.Running = s.Healthy
		if s.Running {
			s.State = "running"
		}
	}
	return s, nil
}
func (c systemWebController) Action(ctx context.Context, action string) (result WebStatus, resultErr error) {
	s, e := c.Status(ctx)
	if e != nil {
		return s, e
	}
	if action == "start" && s.Running || action == "stop" && s.State == "stopped" {
		return s, nil
	}
	if action == "start" {
		if !s.Installed {
			s.ErrorCode = "WEB_NOT_INSTALLED"
			return s, fmt.Errorf("请先安装可选 Web 组件")
		}
		if !s.Configured {
			s.ErrorCode = "WEB_NOT_CONFIGURED"
			return s, fmt.Errorf("请先完成 Web 私有配置")
		}
		if s.ServiceActive {
			s.ErrorCode = "BUSY"
			return s, fmt.Errorf("Web 服务活动但未就绪，请先关闭")
		}
		cfg, _ := webconfig.Load(webconfig.ProductionPath)
		listener, e := net.Listen("tcp", cfg.Listen)
		if e != nil {
			s.ErrorCode = "PORT_IN_USE"
			return s, fmt.Errorf("Web 端口已被占用")
		}
		listener.Close()
	}
	// 只清理本次尝试启动的 Web unit；原来已运行的实例在上面已返回。
	if action == "start" {
		defer func() {
			if resultErr != nil {
				cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if _, err := webCommand(cleanup, "stop", webUnit); err != nil {
					result.Message = "启动未完成，停止检查也未确认，请刷新状态"
				}
			}
		}()
	}
	if _, e = webCommand(ctx, action, webUnit); e != nil {
		return s, fmt.Errorf("Web 服务操作失败")
	}
	for {
		s, e = c.Status(ctx)
		if e == nil && (action == "start" && s.Running || action == "stop" && s.State == "stopped") {
			return s, nil
		}
		select {
		case <-ctx.Done():
			return s, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}
func (d *Daemon) handleWeb(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimPrefix(r.URL.Path, "/v1/web/")
	if (action == "status" && r.Method != "GET") || (action != "status" && r.Method != "POST") {
		writeError(w, 405, fmt.Errorf("方法不允许"))
		return
	}
	if action != "status" && action != "start" && action != "stop" {
		http.NotFound(w, r)
		return
	}
	controller := d.webControl()
	if controller == nil {
		writeJSON(w, 503, map[string]any{"success": false, "error": "此环境未配置 Web 服务控制器", "error_code": "WEB_STATUS_UNAVAILABLE"})
		return
	}
	if action != "status" {
		if _, production := controller.(systemWebController); production {
			unlock, err := lockWebDeployment()
			if err != nil {
				writeJSON(w, 409, map[string]any{"success": false, "error": "Web 正在部署或操作", "error_code": "BUSY"})
				return
			}
			defer unlock()
		}
		if !d.webMu.TryLock() {
			writeJSON(w, 409, map[string]any{"success": false, "error": "Web 操作进行中", "error_code": "BUSY"})
			return
		}
		defer d.webMu.Unlock()
	}
	timeout := 15 * time.Second
	if action == "status" {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	var s WebStatus
	var e error
	if action == "status" {
		s, e = controller.Status(ctx)
	} else {
		s, e = controller.Action(ctx, action)
	}
	if e != nil {
		code, key := 500, "WEB_"+strings.ToUpper(action)+"_FAILED"
		if action == "status" {
			code, key = 503, "WEB_STATUS_UNAVAILABLE"
		}
		if s.ErrorCode != "" {
			code, key = 409, s.ErrorCode
		}
		if ctx.Err() != nil {
			code, key = 504, "RESULT_UNKNOWN"
		}
		writeJSON(w, code, map[string]any{"success": false, "error": e.Error(), "error_code": key})
		return
	}
	writeJSON(w, 200, ok(s))
}
func (c *IPCClient) ManagerWeb(action string) (*WebStatus, error) {
	method := "POST"
	if action == "status" {
		method = "GET"
	}
	if action != "status" && action != "start" && action != "stop" {
		return nil, fmt.Errorf("无效 Web 操作")
	}
	// 独立客户端超时，不改变其他 TUI 请求的等待窗口。
	copyClient := *c
	httpClient := *c.client
	httpClient.Timeout = 20 * time.Second
	copyClient.client = &httpClient
	resp, e := copyClient.requestJSON(method, "/v1/web/"+action, nil, nil)
	if e != nil {
		return nil, e
	}
	s, e := unmarshalData[WebStatus](resp)
	return &s, e
}
