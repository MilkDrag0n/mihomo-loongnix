package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"mihomotui/internal/webconfig"
	"mihomotui/internal/webgateway"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func run() error {
	configPath := flag.String("config", webconfig.ProductionPath, "私有 JSON 配置")
	static := flag.String("static", "", "静态资源目录，默认位于可执行文件旁 static")
	check := flag.Bool("check", false, "只检查配置与静态资源，不启动服务")
	hash := flag.Bool("hash-password", false, "从标准输入读取密码并生成哈希")
	flag.Parse()
	if *hash {
		b, e := io.ReadAll(io.LimitReader(os.Stdin, 1026))
		if e != nil {
			return e
		}
		h, e := webconfig.HashPassword(strings.TrimSuffix(strings.TrimSuffix(string(b), "\n"), "\r"))
		if e != nil {
			return e
		}
		fmt.Println(h)
		return nil
	}
	c, e := webconfig.Load(*configPath)
	if e != nil {
		return e
	}
	if *static == "" {
		binary, e := os.Executable()
		if e != nil {
			return e
		}
		binary, e = filepath.EvalSymlinks(binary)
		if e != nil {
			return e
		}
		*static = filepath.Join(filepath.Dir(binary), "static")
	}
	root, e := filepath.EvalSymlinks(*static)
	if e != nil {
		return fmt.Errorf("静态资源目录不存在")
	}
	if _, e = os.Stat(filepath.Join(root, "index.html")); e != nil {
		return fmt.Errorf("缺少网页构建产物")
	}
	if *check {
		fmt.Println("Web 配置与静态资源检查通过；未启动服务")
		return nil
	}
	app, e := webgateway.New(c, root)
	if e != nil {
		return e
	}
	defer app.Close()
	server := &http.Server{Addr: c.Listen, Handler: app, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	signals, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-signals.Done()
		app.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	fmt.Printf("Mihomo Web 已监听 %s\n", c.Listen)
	e = server.ListenAndServe()
	if e == http.ErrServerClosed {
		return nil
	}
	return e
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "Web 启动失败：", e)
		os.Exit(1)
	}
}
