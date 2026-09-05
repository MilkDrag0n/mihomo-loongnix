package mihomotui

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Daemon IPC 服务端
type Daemon struct {
	mu              sync.RWMutex
	actionMu        sync.Mutex
	delayMu         sync.Mutex
	webMu           sync.Mutex
	web             webController
	listener        net.Listener
	server          *http.Server
	mihomoAPI       *MihomoAPI
	mihomoProcess   *MihomoProcess
	core            coreController
	logRecorder     *ManagedLogRecorder
	upgradeProgress UpgradeProgress

	// 配置应用串行化（P1）：所有运行时应用任务经 reconcileCh 排队逐个执行。
	reconcileOnce               sync.Once
	reconcileCh                 chan reconcileRequest
	reconcileApply              reconcileApplyFunc // 测试注入点；为 nil 时使用 runReconcile
	subscriptionSchedulerCancel context.CancelFunc
}

// RunDaemon 启动 IPC 后台服务
func RunDaemon() error {
	d := &Daemon{}
	return d.Run()
}

// Run 启动守护进程
func (d *Daemon) Run() error {
	// 独立服务端（非一体模式）且 root 用户，使用 /var 路径
	launchMode := os.Getenv("MIHOMO_TUI_LAUNCH_MODE")
	isEmbedded := launchMode == "embedded"
	if !isEmbedded && os.Geteuid() == 0 && GetCustomConfigDir() == "" {
		SetCustomConfigDir("/var/lib/mihomo-tui")
	}

	// 确保配置目录存在
	configDir := GetConfigDir()
	if configDir == "" {
		return fmt.Errorf("配置目录未初始化")
	}
	mixedPort, controller, err := managerRuntimeNetwork()
	if err != nil {
		return err
	}

	// 初始化全局配置（服务端独占）
	cfg := GlobalConfig()
	if cfg.Mihomo.HTTPPort != 0 || cfg.Mihomo.SOCKS5Port != 0 || cfg.Mihomo.MixedPort != mixedPort || cfg.Mihomo.RedirPort != 0 || cfg.Mihomo.TProxyPort != 0 || cfg.Mihomo.ExternalController != controller || cfg.ProxyMode != "rule" || cfg.System.SystemProxy || profilePoolNeedsNormalization(cfg) {
		committed, err := UpdateGlobalConfig(func(c *Config) error {
			c.Mihomo.HTTPPort, c.Mihomo.SOCKS5Port, c.Mihomo.MixedPort = 0, 0, mixedPort
			c.Mihomo.RedirPort, c.Mihomo.TProxyPort = 0, 0
			c.Mihomo.ExternalController = controller
			c.ProxyMode, c.System.SystemProxy = "rule", false
			normalizeProfilePool(c)
			return nil
		})
		if err != nil {
			return fmt.Errorf("迁移精简 Manager 配置失败: %w", err)
		}
		cfg = &committed
	}
	Infof("守护进程启动，配置目录: %s", configDir)

	// 将 LoadConfig 完成的历史订阅池迁移原子写回，保证下一次启动无需再次迁移。
	if len(cfg.Subscriptions) > 0 && len(cfg.SubscriptionPools) > 0 {
		if committed, err := UpdateGlobalConfig(func(c *Config) error { return nil }); err != nil {
			Warnf("持久化订阅池迁移失败: %v", err)
		} else {
			cfg = &committed
		}
	}

	// 确保 API secret 已设置（mihomo external-controller 需要认证）；
	// 通过原子提交持久化，失败时内存与磁盘保持一致。
	if cfg.Mihomo.Secret == "" {
		committed, err := UpdateGlobalConfig(func(c *Config) error {
			if c.Mihomo.Secret == "" {
				c.Mihomo.Secret = generateRandomSecret()
			}
			return nil
		})
		if err != nil {
			Warnf("保存 API secret 失败: %v", err)
		} else {
			cfg = &committed
			Infof("已生成 API secret")
		}
	}

	// 自动创建 mihomo 工作目录
	mihomoDir := filepath.Join(configDir, "mihomo")
	if err := os.MkdirAll(mihomoDir, 0700); err != nil {
		Warnf("创建 mihomo 工作目录失败: %v", err)
	} else if err := os.Chmod(mihomoDir, 0700); err != nil {
		Warnf("收紧 mihomo 工作目录权限失败: %v", err)
	}

	// 初始化 mihomo API 客户端
	d.mihomoAPI = NewMihomoAPIFromConfig()

	// 初始化 mihomo 进程管理器
	d.mihomoProcess = NewMihomoProcess()
	d.ensureCoreController()
	MigrateLegacyMihomoBinary()
	// systemd 只负责拉起管理守护进程；用户启用 AutoStart 后，由 daemon
	// 恢复其管理的 mihomo 子进程。启动失败不应拖垮 IPC 服务，否则用户
	// 无法进入 TUI 修复配置或端口冲突。
	if cfg.System.AutoStart && os.Getenv("MIHOMO_TUI_CORE_SERVICE") == "" {
		if err := d.mihomoProcess.Start(); err != nil {
			Warnf("mihomo 自动启动失败，守护进程继续运行以便修复: %v", err)
		} else {
			Infof("mihomo 已按 auto_start 配置自动启动")
		}
	}
	d.logRecorder = NewManagedLogRecorder(cfg.LogDir, func() (*MihomoAPI, error) {
		d.mu.RLock()
		api := d.mihomoAPI
		d.mu.RUnlock()
		if api == nil {
			return nil, fmt.Errorf("mihomo API 未初始化")
		}
		return api, nil
	})
	if err := d.logRecorder.Apply(cfg.ManagedLogging); err != nil {
		Warnf("恢复磁盘日志记录失败，保持关闭: %v", err)
		_, _ = UpdateGlobalConfig(func(c *Config) error { c.ManagedLogging.Enabled = false; return nil })
	}

	// 初始化 IPC 授权器，并以最小权限创建 socket 目录。root daemon 只允许
	// mihomo-tui 组成员通过 socket 访问；普通 daemon 则只允许启动它的用户访问。
	authorizer, err := newIPCAuthorizer()
	if err != nil {
		return fmt.Errorf("初始化 IPC 授权失败: %w", err)
	}
	sock := daemonSocketPath()
	sockDir := filepath.Dir(sock)
	if socketDirectoryIsUnsafe(sockDir) {
		return fmt.Errorf("拒绝在共享目录 %s 中直接创建 IPC socket；请使用专用子目录", sockDir)
	}
	if err := os.MkdirAll(sockDir, 0750); err != nil {
		return fmt.Errorf("创建 socket 目录失败: %w", err)
	}
	if err := authorizer.configureSocketDirectory(sockDir); err != nil {
		return err
	}

	// 清理旧 socket
	if err := os.Remove(sock); err != nil && !os.IsNotExist(err) {
		// 如果无法删除，检查是否已有 daemon 在监听
		if conn, dialErr := net.Dial("unix", sock); dialErr == nil {
			conn.Close()
			return fmt.Errorf("IPC 服务已在运行: %s", sock)
		}
		// 无法删除且没有 daemon 在监听，可能是权限问题
		return fmt.Errorf("无法清理旧 socket %s: %w", sock, err)
	}

	// 创建 UDS listener
	listener, err := net.Listen("unix", sock)
	if err != nil {
		return fmt.Errorf("监听 Unix socket %s 失败: %w", sock, err)
	}
	defer listener.Close()

	if err := authorizer.configureSocketPermissions(sock); err != nil {
		return fmt.Errorf("设置 IPC socket 权限失败: %w", err)
	}

	d.listener = listener
	d.server = &http.Server{
		Handler:     authorizer.middleware(d.router()),
		ConnContext: authorizer.connContext,
	}

	Infof("IPC 服务已启动: %s", sock)
	fmt.Printf("[mihomo-tui server] IPC 服务已启动: %s\n", sock)
	fmt.Println("[mihomo-tui server] 按 Ctrl+C 停止服务")
	return d.server.Serve(listener)
}

func socketDirectoryIsUnsafe(dir string) bool {
	dir = filepath.Clean(dir)
	for _, shared := range []string{"/", "/tmp", "/run", "/var/run", filepath.Clean(os.TempDir())} {
		if dir == shared {
			return true
		}
	}
	return false
}

// managerRuntimeNetwork keeps the controller fixed and uses the persisted
// managed mixed port in production. Explicit shadow mode remains isolated from
// production configuration so upgrades cannot touch live listeners.
func managerRuntimeNetwork() (int, string, error) {
	if os.Getenv("MIHOMO_TUI_SHADOW") != "1" {
		mixedPort := GlobalConfig().Mihomo.MixedPort
		if mixedPort == 0 {
			mixedPort = 7890
		}
		if mixedPort < 1 || mixedPort > 65535 || mixedPort == 9090 {
			return 0, "", fmt.Errorf("受管 mixed 端口必须在 1-65535 之间且不能是控制端口 9090")
		}
		return mixedPort, "127.0.0.1:9090", nil
	}
	mixedPort, controllerPort := 17890, 19090
	for name, target := range map[string]*int{
		"MIHOMO_TUI_SHADOW_MIXED_PORT":      &mixedPort,
		"MIHOMO_TUI_SHADOW_CONTROLLER_PORT": &controllerPort,
	} {
		if raw := strings.TrimSpace(os.Getenv(name)); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 65535 {
				return 0, "", fmt.Errorf("%s 必须是 1-65535 的端口", name)
			}
			*target = value
		}
	}
	if mixedPort == controllerPort {
		return 0, "", fmt.Errorf("影子 mixed 与 controller 端口不能相同")
	}
	return mixedPort, net.JoinHostPort("127.0.0.1", strconv.Itoa(controllerPort)), nil
}

// Stop 停止守护进程
func (d *Daemon) Stop() error {
	if d.subscriptionSchedulerCancel != nil {
		d.subscriptionSchedulerCancel()
	}
	if d.logRecorder != nil {
		d.logRecorder.Close()
	}
	if d.server != nil {
		return d.server.Shutdown(context.Background())
	}
	return nil
}

// router 返回 HTTP 路由
func (d *Daemon) router() http.Handler {
	mux := http.NewServeMux()

	// 精简 Manager v1：TUI 只调用这些权威接口，不直接连接 mihomo。
	mux.HandleFunc("/v1/status", d.handleManagerStatus)
	mux.HandleFunc("/v1/web/", d.handleWeb)
	mux.HandleFunc("/v1/core/start", d.handleManagerCoreStart)
	mux.HandleFunc("/v1/core/stop", d.handleManagerCoreStop)
	mux.HandleFunc("/v1/tun", d.handleManagerTUN)
	mux.HandleFunc("/v1/proxy-port", d.handleManagerProxyPort)
	mux.HandleFunc("/v1/profiles", d.handleProfiles)
	mux.HandleFunc("/v1/profiles/", d.handleProfileDetail)
	mux.HandleFunc("/v1/proxy-groups", d.handleManagerProxyGroups)
	mux.HandleFunc("/v1/proxy-groups/", d.handleManagerProxyGroupDetail)
	mux.HandleFunc("/v1/proxy-delay", d.handleManagerProxyDelay)
	mux.HandleFunc("/v1/rules", d.handleManagerRules)
	mux.HandleFunc("/v1/logs/stream", d.handleManagerLogStream)
	mux.HandleFunc("/v1/logging/status", d.handleManagerLoggingStatus)
	mux.HandleFunc("/v1/logging", d.handleManagerLogging)

	// 心跳
	mux.HandleFunc("/api/v1/ping", d.handlePing)

	// 守护进程信息
	mux.HandleFunc("/api/v1/daemon/info", d.handleDaemonInfo)
	mux.HandleFunc("/api/v1/daemon/config-dir", d.handleDaemonConfigDir)

	return mux
}

// ========== 辅助函数 ==========

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, APIResponse{Success: false, Error: err.Error()})
}

func readJSON(r *http.Request, dest any) error {
	return json.NewDecoder(r.Body).Decode(dest)
}

func ok(data any) APIResponse {
	return APIResponse{Success: true, Data: data}
}

// ========== 守护进程自身 Handler ==========

func (d *Daemon) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("方法不允许"))
		return
	}
	writeJSON(w, http.StatusOK, ok("pong"))
}

func (d *Daemon) handleDaemonInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("方法不允许"))
		return
	}
	launchMode := os.Getenv("MIHOMO_TUI_LAUNCH_MODE")
	if launchMode == "" {
		launchMode = "standalone"
	}
	info := DaemonInfo{
		LaunchMode:         launchMode,
		IsRoot:             os.Geteuid() == 0,
		CanManageMihomo:    requestIPCRole(r) == ipcRoleAdmin,
		CanManageResources: requestIPCRole(r) >= ipcRoleOperator,
	}
	writeJSON(w, http.StatusOK, ok(info))
}

func (d *Daemon) handleDaemonConfigDir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("方法不允许"))
		return
	}
	writeJSON(w, http.StatusOK, ok(map[string]string{"config_dir": GetConfigDir()}))
}

func (d *Daemon) handleDaemonShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("方法不允许"))
		return
	}
	writeJSON(w, http.StatusOK, ok("shutdown"))
	// 在响应发送后异步停止 server
	go func() {
		time.Sleep(100 * time.Millisecond)
		if err := d.Stop(); err != nil {
			Errorf("守护进程停止失败: %v", err)
		}
	}()
}
