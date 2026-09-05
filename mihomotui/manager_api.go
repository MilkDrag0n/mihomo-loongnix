package mihomotui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type CoreRuntimeStatus struct {
	StateQueryOK      bool   `json:"state_query_ok"`
	ServiceState      string `json:"service_state"`
	ServiceActive     bool   `json:"service_active"`
	ControllerHealthy bool   `json:"controller_healthy"`
	Running           bool   `json:"running"`
	PID               int    `json:"pid"`
	Detail            string `json:"detail,omitempty"`
}

type TUNRuntimeStatus struct {
	ObservationOK    bool   `json:"observation_ok"`
	Configured       bool   `json:"configured"`
	RuntimeEnabled   bool   `json:"runtime_enabled"`
	InterfacePresent bool   `json:"interface_present"`
	Enabled          bool   `json:"enabled"`
	Interface        string `json:"interface"`
}

type ManagerStatus struct {
	ObservedAt    string            `json:"observed_at"`
	Core          CoreRuntimeStatus `json:"core"`
	TUN           TUNRuntimeStatus  `json:"tun"`
	ActiveProfile *ProfileSummary   `json:"active_profile,omitempty"`
	ProxyPort     int               `json:"proxy_port"`
	CurrentGroup  string            `json:"current_group,omitempty"`
	CurrentNode   string            `json:"current_node,omitempty"`
}

type TUNSetRequest struct {
	Enabled bool `json:"enabled"`
}

type ProxyPortSetRequest struct {
	Port int `json:"port"`
}

type LoggingSetRequest struct {
	Enabled bool `json:"enabled"`
}

type ProxyDelayTestRequest struct {
	Group string `json:"group"`
	Name  string `json:"name"`
}

func (d *Daemon) managerStatus() (ManagerStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	state, stateErr := d.ensureCoreController().State(ctx)
	status := ManagerStatus{Core: CoreRuntimeStatus{ServiceActive: state.Active, PID: state.PID, Detail: state.Detail}}
	if stateErr != nil {
		status.Core.Detail = RedactURLInText(stateErr.Error())
	}
	status.Core.StateQueryOK = stateErr == nil
	status.Core.ServiceState = "unknown"
	if stateErr == nil {
		switch state.Detail {
		case "active", "inactive", "failed", "activating", "deactivating":
			status.Core.ServiceState = state.Detail
		default:
			if state.Active {
				status.Core.ServiceState = "active"
			} else {
				status.Core.ServiceState = "inactive"
			}
		}
	}
	cfg := GlobalConfig()
	status.CurrentGroup = cfg.defaultProxyGroup()
	if mixedPort, _, err := managerRuntimeNetwork(); err == nil {
		status.ProxyPort = mixedPort
	}
	if cfg.ActiveSubscription >= 0 && cfg.ActiveSubscription < len(cfg.Subscriptions) {
		profile := profileSummary(cfg, cfg.Subscriptions[cfg.ActiveSubscription])
		status.ActiveProfile = &profile
	}
	status.TUN.Configured = cfg.System.TUN
	status.TUN.Interface = "mihomo-tui-tun"
	interfaces, interfaceErr := net.Interfaces()
	for _, item := range interfaces {
		if item.Name == status.TUN.Interface {
			status.TUN.InterfacePresent = true
		}
	}
	status.TUN.ObservationOK = stateErr == nil && !state.Active && interfaceErr == nil
	if state.Active {
		api := NewMihomoAPIFromConfig()
		if _, err := api.GetVersion(); err == nil {
			status.Core.ControllerHealthy = true
			var configOK bool
			status.TUN.RuntimeEnabled, status.ProxyPort, configOK = runtimeConfigState(api, status.ProxyPort)
			status.TUN.ObservationOK = configOK && interfaceErr == nil
			status.CurrentNode = runtimeCurrentNode(api, status.CurrentGroup)
		} else if status.Core.Detail == "" {
			status.Core.Detail = RedactURLInText(err.Error())
		}
	}
	status.Core.Running = status.Core.ServiceActive && status.Core.ControllerHealthy
	status.TUN.Enabled = status.Core.Running && status.TUN.RuntimeEnabled && status.TUN.InterfacePresent
	status.ObservedAt = time.Now().UTC().Format(time.RFC3339)
	return status, stateErr
}

func interfaceExists(name string) bool {
	interfaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, item := range interfaces {
		if item.Name == name {
			return true
		}
	}
	return false
}

func runtimeConfigState(api *MihomoAPI, fallbackPort int) (bool, int, bool) {
	data, err := api.GetConfigs()
	if err != nil {
		return false, fallbackPort, false
	}
	var body map[string]any
	if json.Unmarshal(data, &body) != nil {
		return false, fallbackPort, false
	}
	tun, ok := body["tun"].(map[string]any)
	enabled := false
	if ok {
		enabled, _ = tun["enable"].(bool)
	}
	port := fallbackPort
	for _, key := range []string{"mixed-port", "mixed_port"} {
		if value, exists := body[key].(float64); exists && value >= 1 && value <= 65535 {
			port = int(value)
			break
		}
	}
	return enabled, port, ok
}

func runtimeMixedPort(api *MihomoAPI) (int, error) {
	data, err := api.GetConfigs()
	if err != nil {
		return 0, err
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		return 0, err
	}
	for _, key := range []string{"mixed-port", "mixed_port"} {
		if value, exists := body[key].(float64); exists && value >= 1 && value <= 65535 {
			return int(value), nil
		}
	}
	return 0, fmt.Errorf("Mihomo 运行配置未返回 mixed-port")
}

func runtimeCurrentNode(api *MihomoAPI, group string) string {
	if strings.TrimSpace(group) == "" {
		return ""
	}
	data, err := api.GetProxy(group)
	if err != nil {
		return ""
	}
	var proxy mihomoProxy
	if json.Unmarshal(data, &proxy) != nil {
		return ""
	}
	return strings.TrimSpace(proxy.Now)
}

func (d *Daemon) handleManagerStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("方法不允许"))
		return
	}
	status, _ := d.managerStatus()
	writeJSON(w, http.StatusOK, ok(status))
}

func (d *Daemon) handleManagerCoreStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("方法不允许"))
		return
	}
	d.actionMu.Lock()
	defer d.actionMu.Unlock()
	cfg := GlobalConfig()
	if err := cfg.GenerateMihomoConfig(); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("生成运行配置失败: %w", err))
		return
	}
	if err := validateGeneratedMihomoConfig(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if cfg.System.TUN {
		if err := SetupTUNRouting(); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("TUN 安全预检失败: %w", err))
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	if err := d.ensureCoreController().Start(ctx); err != nil {
		if cfg.System.TUN {
			_ = RestoreTUNRouting()
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	status, err := waitForCore(ctx, true, d.managerStatus)
	if err != nil {
		_ = d.ensureCoreController().Stop(context.Background())
		if cfg.System.TUN {
			_ = RestoreTUNRouting()
		}
		writeError(w, http.StatusGatewayTimeout, err)
		return
	}
	writeJSON(w, http.StatusOK, ok(status))
}

func (d *Daemon) handleManagerCoreStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("方法不允许"))
		return
	}
	d.actionMu.Lock()
	defer d.actionMu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	before, _ := d.managerStatus()
	state, _ := d.ensureCoreController().State(ctx)
	if state.Active {
		if err := d.ensureCoreController().Stop(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if tunCleanupNeeded(before.TUN) {
		if err := RestoreTUNRouting(); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("内核已停止，但清理 TUN 路由失败: %w", err))
			return
		}
	}
	status, err := waitForCore(ctx, false, d.managerStatus)
	if err != nil {
		writeError(w, http.StatusGatewayTimeout, err)
		return
	}
	writeJSON(w, http.StatusOK, ok(status))
}

func (d *Daemon) handleManagerTUN(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("方法不允许"))
		return
	}
	var req TUNSetRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("请求无效: %w", err))
		return
	}
	d.actionMu.Lock()
	defer d.actionMu.Unlock()
	old := GlobalConfig().System.TUN
	if old == req.Enabled {
		status, _ := d.managerStatus()
		writeJSON(w, http.StatusOK, ok(status))
		return
	}
	initialStatus, _ := d.managerStatus()
	if initialStatus.Core.ServiceActive && !initialStatus.Core.ControllerHealthy {
		writeError(w, http.StatusConflict, fmt.Errorf("Mihomo 服务活动但控制接口不健康，拒绝在未知运行状态下修改 TUN"))
		return
	}
	if _, err := UpdateGlobalConfig(func(c *Config) error { c.System.TUN = req.Enabled; c.ProxyMode = "rule"; return nil }); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	rollback := func() {
		_, _ = UpdateGlobalConfig(func(c *Config) error { c.System.TUN = old; return nil })
		cfg := GlobalConfig()
		_ = cfg.GenerateMihomoConfig()
		api := NewMihomoAPIFromConfig()
		_ = api.ReloadConfigs(true)
		if old {
			_ = SetupTUNRouting()
		} else {
			_ = RestoreTUNRouting()
		}
	}
	cfg := GlobalConfig()
	if err := cfg.GenerateMihomoConfig(); err != nil {
		rollback()
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	status, _ := d.managerStatus()
	if status.Core.Running {
		if req.Enabled {
			if err := SetupTUNRouting(); err != nil {
				rollback()
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
		if err := NewMihomoAPIFromConfig().ReloadConfigs(true); err != nil {
			rollback()
			writeError(w, http.StatusInternalServerError, fmt.Errorf("热重载失败: %w", err))
			return
		}
		if !req.Enabled {
			if err := RestoreTUNRouting(); err != nil {
				rollback()
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		for {
			status, _ = d.managerStatus()
			matches := status.TUN.Enabled
			if !req.Enabled {
				matches = !status.TUN.Configured && !status.TUN.RuntimeEnabled && !status.TUN.InterfacePresent
			}
			if matches {
				break
			}
			select {
			case <-ctx.Done():
				rollback()
				writeError(w, http.StatusGatewayTimeout, fmt.Errorf("TUN 状态回读未达到目标，已回滚"))
				return
			case <-time.After(150 * time.Millisecond):
			}
		}
	} else if !req.Enabled && tunCleanupNeeded(status.TUN) {
		if err := RestoreTUNRouting(); err != nil {
			rollback()
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	status, _ = d.managerStatus()
	writeJSON(w, http.StatusOK, ok(status))
}

func (d *Daemon) handleManagerProxyPort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("方法不允许"))
		return
	}
	var req ProxyPortSetRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("请求无效: %w", err))
		return
	}
	if req.Port < 1 || req.Port > 65535 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("代理端口必须在 1-65535 之间"))
		return
	}

	d.actionMu.Lock()
	defer d.actionMu.Unlock()
	before := GlobalConfig().Clone()
	if req.Port == before.Mihomo.MixedPort {
		status, _ := d.managerStatus()
		writeJSON(w, http.StatusOK, ok(status))
		return
	}
	if _, rawPort, err := net.SplitHostPort(before.Mihomo.ExternalController); err == nil {
		if controllerPort, _ := strconv.Atoi(rawPort); controllerPort == req.Port {
			writeError(w, http.StatusConflict, fmt.Errorf("代理端口不能与控制端口 %d 相同", controllerPort))
			return
		}
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(req.Port)))
	if err != nil {
		writeError(w, http.StatusConflict, fmt.Errorf("端口 %d 已被占用或不可用", req.Port))
		return
	}
	_ = listener.Close()

	initial, _ := d.managerStatus()
	if initial.Core.ServiceActive && !initial.Core.ControllerHealthy {
		writeError(w, http.StatusConflict, fmt.Errorf("Mihomo 服务活动但控制接口不健康，拒绝在未知运行状态下修改端口"))
		return
	}
	rollback := func() {
		_ = restoreConfigSnapshot(before)
		old := GlobalConfig()
		_ = old.GenerateMihomoConfig()
		if initial.Core.Running {
			_ = NewMihomoAPIFromConfig().ReloadConfigs(true)
		}
	}
	if _, err := UpdateGlobalConfig(func(c *Config) error {
		c.Mihomo.HTTPPort = 0
		c.Mihomo.SOCKS5Port = 0
		c.Mihomo.MixedPort = req.Port
		return nil
	}); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cfg := GlobalConfig()
	if err := cfg.GenerateMihomoConfig(); err != nil {
		rollback()
		writeError(w, http.StatusInternalServerError, fmt.Errorf("生成运行配置失败，已回滚: %w", err))
		return
	}
	if initial.Core.Running {
		api := NewMihomoAPIFromConfig()
		if err := api.ReloadConfigs(true); err != nil {
			rollback()
			writeError(w, http.StatusInternalServerError, fmt.Errorf("应用代理端口失败，已回滚: %w", err))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		for {
			actual, readErr := runtimeMixedPort(api)
			if readErr == nil && actual == req.Port {
				break
			}
			select {
			case <-ctx.Done():
				rollback()
				writeError(w, http.StatusGatewayTimeout, fmt.Errorf("端口状态回读未达到 %d，已回滚", req.Port))
				return
			case <-time.After(150 * time.Millisecond):
			}
		}
	}
	status, _ := d.managerStatus()
	if status.ProxyPort != req.Port {
		rollback()
		writeError(w, http.StatusConflict, fmt.Errorf("端口状态不一致，已回滚"))
		return
	}
	writeJSON(w, http.StatusOK, ok(status))
}

func (d *Daemon) handleManagerProxyGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("方法不允许"))
		return
	}
	groups, err := NewMihomoAPIFromConfig().GetProxyGroups()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, ok(groups))
}

func (d *Daemon) handleManagerProxyGroupDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("方法不允许"))
		return
	}
	groupName := strings.TrimPrefix(r.URL.Path, "/v1/proxy-groups/")
	if strings.TrimSpace(groupName) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("代理组无效"))
		return
	}
	var req ProxySelectRequest
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("节点名称不能为空"))
		return
	}
	d.actionMu.Lock()
	defer d.actionMu.Unlock()
	api := NewMihomoAPIFromConfig()
	if err := api.SelectProxy(groupName, req.Name); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	groups, err := api.GetProxyGroups()
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("选择已发送但状态回读失败: %w", err))
		return
	}
	for _, group := range groups {
		if group.Name == groupName {
			if group.Now != req.Name {
				writeError(w, http.StatusConflict, fmt.Errorf("内核实际节点为 %q，未切换到 %q", group.Now, req.Name))
				return
			}
			writeJSON(w, http.StatusOK, ok(group))
			return
		}
	}
	writeError(w, http.StatusNotFound, fmt.Errorf("代理组不存在"))
}

func (d *Daemon) handleManagerProxyDelay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("方法不允许"))
		return
	}
	var req ProxyDelayTestRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("请求无效: %w", err))
		return
	}
	if strings.TrimSpace(req.Group) == "" || strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("代理组和节点名称不能为空"))
		return
	}

	// 节点测速可能持续数秒，单独串行化，不阻塞内核、TUN 和配置操作。
	d.delayMu.Lock()
	defer d.delayMu.Unlock()
	status, _ := d.managerStatus()
	if !status.Core.Running {
		writeError(w, http.StatusConflict, fmt.Errorf("Mihomo 内核未运行或控制接口不健康"))
		return
	}
	api := NewMihomoAPIFromConfig()
	groups, err := api.GetProxyGroups()
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("读取代理组失败: %w", err))
		return
	}
	foundGroup, foundNode := false, false
	for _, group := range groups {
		if group.Name != req.Group {
			continue
		}
		foundGroup = true
		for _, node := range group.Nodes {
			if node.Name == req.Name {
				foundNode = true
				break
			}
		}
		break
	}
	if !foundGroup {
		writeError(w, http.StatusNotFound, fmt.Errorf("代理组不存在"))
		return
	}
	if !foundNode {
		writeError(w, http.StatusNotFound, fmt.Errorf("节点不在指定代理组中"))
		return
	}
	testURL := strings.TrimSpace(GlobalConfig().Mihomo.TestURL)
	if testURL == "" {
		testURL = "http://cp.cloudflare.com/generate_204"
	}
	delay, err := api.TestProxyDelayValue(req.Name, testURL, 5000)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("节点测速失败: %w", err))
		return
	}
	if delay <= 0 {
		delay = DelayTimeout
	}
	writeJSON(w, http.StatusOK, ok(ProxyDelayResponse{Name: req.Name, Delay: delay}))
}

func (d *Daemon) handleManagerRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("方法不允许"))
		return
	}
	rules, err := NewMihomoAPIFromConfig().GetRulesParsed()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, ok(rules))
}

func (d *Daemon) handleManagerLogStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("方法不允许"))
		return
	}
	resp, err := NewMihomoAPIFromConfig().GetLogsStreamContext(r.Context(), r.URL.Query().Get("level"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Errorf("mihomo 日志接口返回 %s", resp.Status))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	buffer := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			if _, err := w.Write(buffer[:n]); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				Debugf("实时日志流结束: %v", readErr)
			}
			return
		}
	}
}

func (d *Daemon) ensureLogRecorder() *ManagedLogRecorder {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.logRecorder == nil {
		d.logRecorder = NewManagedLogRecorder(GlobalConfig().LogDir, func() (*MihomoAPI, error) { return NewMihomoAPIFromConfig(), nil })
		_ = d.logRecorder.Apply(ManagedLoggingConfig{Enabled: false, MaxFileBytes: GlobalConfig().ManagedLogging.MaxFileBytes, MaxBackups: GlobalConfig().ManagedLogging.MaxBackups})
	}
	return d.logRecorder
}

func (d *Daemon) handleManagerLoggingStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("方法不允许"))
		return
	}
	writeJSON(w, http.StatusOK, ok(d.ensureLogRecorder().Status()))
}

func (d *Daemon) handleManagerLogging(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("方法不允许"))
		return
	}
	var req LoggingSetRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	d.actionMu.Lock()
	defer d.actionMu.Unlock()
	previous := GlobalConfig().ManagedLogging
	committed, err := UpdateGlobalConfig(func(c *Config) error { c.ManagedLogging.Enabled = req.Enabled; return nil })
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := d.ensureLogRecorder().Apply(committed.ManagedLogging); err != nil {
		_, _ = UpdateGlobalConfig(func(c *Config) error { c.ManagedLogging = previous; return nil })
		_ = d.ensureLogRecorder().Apply(previous)
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, ok(d.ensureLogRecorder().Status()))
}

func managedCoreBinaryPath() string {
	if path := findMihomoBinary(); path != "" {
		return path
	}
	return filepath.Join(GetConfigDir(), "bin", "mihomo")
}

func validateCoreFiles() error {
	cfg := GlobalConfig()
	for _, path := range []string{managedCoreBinaryPath(), cfg.MihomoConfigPath} {
		if _, err := os.Stat(path); err != nil {
			return err
		}
	}
	return nil
}

// validateGeneratedMihomoConfig asks the installed core to parse the exact
// generated file before systemd start or hot reload. Structural YAML checks at
// import time catch malformed subscriptions; this final gate catches semantic
// incompatibilities in the concrete LoongArch core build.
func validateGeneratedMihomoConfig() error {
	binary := managedCoreBinaryPath()
	cfg := GlobalConfig()
	if _, err := os.Stat(binary); err != nil {
		return fmt.Errorf("Mihomo 内核不可用，无法验证配置: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, "-t", "-d", GetConfigDir(), "-f", cfg.MihomoConfigPath).CombinedOutput()
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(RedactURLInText(string(output)))
	if len(detail) > 4096 {
		detail = detail[len(detail)-4096:]
	}
	if ctx.Err() != nil {
		if detail != "" {
			return fmt.Errorf("Mihomo 配置验证超时: %w: %s", ctx.Err(), detail)
		}
		return fmt.Errorf("Mihomo 配置验证超时: %w", ctx.Err())
	}
	if detail == "" {
		return fmt.Errorf("Mihomo 拒绝配置: %w", err)
	}
	return fmt.Errorf("Mihomo 拒绝配置: %w: %s", err, detail)
}
