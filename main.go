package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"mihomotui/cmd"
)

// ensureLinuxAdminPath adds the conventional sbin directories used by
// Loongnix for nft/ip/iptables. Interactive Loongnix users do not necessarily
// receive these paths, while systemd services usually do; normalizing PATH
// keeps diagnostics and the daemon consistent without overriding user entries.
func ensureLinuxAdminPath() {
	if runtime.GOOS != "linux" {
		return
	}
	parts := filepath.SplitList(os.Getenv("PATH"))
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		seen[part] = true
	}
	for _, dir := range []string{"/usr/local/sbin", "/usr/sbin", "/sbin"} {
		if !seen[dir] {
			parts = append(parts, dir)
		}
	}
	_ = os.Setenv("PATH", strings.Join(parts, string(os.PathListSeparator)))
}

func main() {
	ensureLinuxAdminPath()
	dir := flag.String("d", "", "指定配置目录")
	standalone := flag.Bool("standalone", false, "启动嵌入式服务端（一体模式）")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "mihomo-tui — mihomo 终端 UI 配置工具")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "用法:")
		fmt.Fprintln(os.Stderr, "  mihomo-tui [选项]              启动 TUI 客户端")
		fmt.Fprintln(os.Stderr, "  mihomo-tui server [选项]       启动后台 IPC 服务")
		fmt.Fprintln(os.Stderr, "  mihomo-tui install_service     安装为 systemd 服务（需 root）")
		fmt.Fprintln(os.Stderr, "  mihomo-tui uninstall           卸载 systemd 服务（需 root）")
		fmt.Fprintln(os.Stderr, "  mihomo-tui grant_operator 用户  授予普通用户订阅管理权限（需 root）")
		fmt.Fprintln(os.Stderr, "  mihomo-tui cleanup             清理 TUN 与旧版遗留代理环境（需 root）")
		fmt.Fprintln(os.Stderr, "  mihomo-tui tun_diagnose        输出 TUN 路由 dry-run 计划（不修改系统）")
		fmt.Fprintln(os.Stderr, "  mihomo-tui tun_debug [--apply] 输出 TUN 预检；--apply 时重建修复并输出日志（需 root）")
		fmt.Fprintln(os.Stderr, "  mihomo-tui web status|start|stop 查询、开启或关闭可选网页")
		fmt.Fprintln(os.Stderr, "  mihomo-tui version             显示版本信息")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "选项:")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()

	// 无子命令：启动 TUI
	if len(args) == 0 {
		cmd.RunTUI(*dir, *standalone)
		return
	}

	// 子命令分发
	switch args[0] {
	case "web":
		if err := cmd.RunWeb(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "server":
		cmd.RunServer(args[1:], *dir)
	case "install_service":
		cmd.RunInstallService(*dir)
	case "uninstall":
		cmd.RunUninstallService()
	case "grant_operator":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "用法: mihomo-tui grant_operator 用户名")
			os.Exit(1)
		}
		cmd.RunGrantOperator(args[1])
	case "cleanup":
		cmd.RunCleanup()
	case "tun_diagnose":
		cmd.RunTUNDiagnose()
	case "tun_debug":
		cmd.RunTUNDebug(*dir, args[1:])
	case "version":
		cmd.RunVersion()
	case "help":
		flag.Usage()
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", args[0])
		flag.Usage()
		os.Exit(1)
	}
}
