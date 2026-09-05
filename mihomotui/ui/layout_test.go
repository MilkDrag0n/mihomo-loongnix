package ui

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"mihomotui/mihomotui"
)

type previewCell struct {
	Text        string
	X, Y, Width int
	FG, BG      string
	Attr        int
}
type terminalFrame struct {
	Width, Height int
	Text          string
	Cells         []previewCell
}

func captureTerminal(screen tcell.Screen) terminalFrame {
	w, h := screen.Size()
	frame := terminalFrame{Width: w, Height: h}
	var text strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; {
			r, comb, style, width := screen.GetContent(x, y)
			if width < 1 {
				width = 1
			}
			chars := string(r) + string(comb)
			text.WriteString(chars)
			fg, bg, attr := style.Decompose()
			frame.Cells = append(frame.Cells, previewCell{chars, x, y, width, fg.String(), bg.String(), int(attr)})
			x += width
		}
		text.WriteByte('\n')
	}
	frame.Text = text.String()
	return frame
}

// 真正运行五页界面，但只连接临时 Unix socket 上的假数据服务。
func TestTerminalLayoutAndModalIsolation(t *testing.T) {
	dir, err := os.MkdirTemp("", "mui-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "daemon.sock")
	t.Setenv("MIHOMO_TUI_SOCKET", socket)
	t.Setenv("MIHOMO_TUI_SHADOW", "1")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	status := mihomotui.ManagerStatus{
		Core:      mihomotui.CoreRuntimeStatus{Running: true, ServiceActive: true, ControllerHealthy: true, PID: 4242},
		ProxyPort: 17890, CurrentGroup: "Auto", CurrentNode: "HK 香港示例 01",
		ActiveProfile: &mihomotui.ProfileSummary{ID: "demo", Name: "演示配置"},
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "测试不允许系统操作", 403)
			return
		}
		var data any
		switch r.URL.Path {
		case "/v1/status":
			data = status
		case "/v1/profiles":
			data = []mihomotui.ProfileSummary{{ID: "demo", Name: "演示配置", Active: true, Source: "example.com", UpdatedAt: "2026-09-05"}}
		case "/v1/proxy-groups":
			data = []mihomotui.ProxyGroup{{Name: "Auto", Type: "Selector", Now: "HK 香港示例 01", Nodes: []mihomotui.ProxyNode{{Name: "HK 香港示例 01", Type: "ss", Delay: 32}, {Name: "JP 东京示例 02", Type: "vmess", Delay: 68}}}}
		case "/v1/rules":
			data = []mihomotui.Rule{{Content: "example.com", Type: "Domain", Policy: "Auto"}, {Content: "MATCH", Type: "Match", Policy: "DIRECT"}}
		case "/v1/logging/status":
			data = mihomotui.LoggingStatus{MaxFileBytes: 10 << 20, MaxBackups: 5}
		case "/v1/logs/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"type\":\"info\",\"payload\":\"演示日志：界面已连接测试后台\"}\n\n")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		default:
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(mihomotui.APIResponse{Success: true, Data: data})
	})}
	go srv.Serve(listener)
	defer srv.Close()
	client, err := mihomotui.NewIPCClient()
	if err != nil {
		t.Fatal(err)
	}
	app, root, start, stop := newClientApplication(client)
	screen := tcell.NewSimulationScreen("UTF-8")
	app.SetScreen(screen)
	screen.SetSize(80, 24)
	frames := make(chan terminalFrame, 64)
	app.SetAfterDrawFunc(func(s tcell.Screen) {
		select {
		case frames <- captureTerminal(s):
		default:
		}
	})
	done := make(chan error, 1)
	go func() { done <- app.Run() }()
	defer func() {
		stop()
		app.Stop()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	start()
	wait := func(contains string) terminalFrame {
		t.Helper()
		timeout := time.NewTimer(5 * time.Second)
		defer timeout.Stop()
		var last terminalFrame
		for {
			select {
			case frame := <-frames:
				last = frame
				if strings.Contains(frame.Text, contains) {
					return frame
				}
			case <-timeout.C:
				t.Fatalf("界面未显示 %q:\n%s", contains, last.Text)
				return terminalFrame{}
			}
		}
	}
	key := func(k tcell.Key, r rune) {
		app.QueueUpdateDraw(func() {
			event := app.GetInputCapture()(tcell.NewEventKey(k, r, tcell.ModNone))
			if event != nil {
				root.InputHandler()(event, func(p tview.Primitive) { app.SetFocus(p) })
			}
		})
	}
	save := func(name string, frame terminalFrame) {
		t.Helper()
		if out := os.Getenv("MIHOMO_TUI_PREVIEW_DIR"); out != "" {
			if err := os.MkdirAll(out, 0700); err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(frame)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, name+".json"), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	frame := wait("演示配置")
	for _, text := range []string{"1 首页", "5 日志", "代理内核", "TUN 网络", "当前配置", "停止代理", "应用端口", "q 退出"} {
		if !strings.Contains(frame.Text, text) {
			t.Fatalf("80×24 缺少 %q:\n%s", text, frame.Text)
		}
	}
	// 空白区域必须真正使用终端默认背景，而不是黑色或固定 RGB。
	for _, cell := range frame.Cells {
		if cell.Y == 4 && cell.BG != tcell.ColorDefault.String() {
			t.Fatalf("背景被覆盖: %+v", cell)
		}
	}
	save("首页-80x24", frame)
	// 在端口框输入无效值打开弹窗；q、Tab 不得关闭程序或穿透到主页。
	key(tcell.KeyTab, 0)
	key(tcell.KeyTab, 0)
	app.QueueUpdateDraw(func() {
		if _, ok := app.GetFocus().(*tview.Button); !ok {
			t.Error("Tab 未离开输入框")
		}
	})
	key(tcell.KeyBacktab, 0)
	app.QueueUpdateDraw(func() {
		field, ok := app.GetFocus().(*tview.TextArea)
		if ok {
			field.SetText("0", false)
		} else {
			t.Errorf("port focus: %T", app.GetFocus())
		}
	})
	key(tcell.KeyEnter, 0)
	wait("端口无效")
	key(tcell.KeyRune, 'q')
	wait("端口无效")
	key(tcell.KeyTab, 0)
	wait("端口无效")
	var modalFocus bool
	app.QueueUpdateDraw(func() { _, modalFocus = app.GetFocus().(*tview.Button) })
	if !modalFocus {
		t.Fatal("Tab 焦点离开弹窗")
	}
	key(tcell.KeyEsc, 0)
	app.QueueUpdateDraw(func() {
		if _, ok := app.GetFocus().(*tview.TextArea); !ok {
			t.Error("Esc 未恢复原焦点")
		}
	})
	key(tcell.KeyEsc, 0)
	for _, page := range []struct {
		key        rune
		want, name string
	}{{'2', "演示配置", "配置"}, {'3', "HK 香港示例 01", "节点"}, {'4', "example.com", "规则"}, {'5', "轮转:", "日志"}} {
		// 排空上页快照，避免误用旧内容。
		app.QueueUpdateDraw(func() {
			for len(frames) > 0 {
				<-frames
			}
		})
		key(tcell.KeyRune, page.key)
		frame = wait(page.want)
		save(page.name+"-80x24", frame)
		if page.key == '3' {
			key(tcell.KeyBacktab, 0)
			key(tcell.KeyBacktab, 0)
			key(tcell.KeyBacktab, 0)
			var drop *tview.DropDown
			app.QueueUpdateDraw(func() { drop, _ = app.GetFocus().(*tview.DropDown) })
			if drop == nil {
				t.Fatal("未聚焦代理组")
			}
			key(tcell.KeyEnter, 0)
			key(tcell.KeyRune, 'q')
			app.QueueUpdateDraw(func() {
				if !drop.IsOpen() {
					t.Error("全局快捷键穿透下拉框")
				}
			})
			key(tcell.KeyEsc, 0)
			app.QueueUpdateDraw(func() {
				if drop.IsOpen() {
					t.Error("Esc 未关闭下拉框")
				}
			})
		}
	}
	key(tcell.KeyRune, '1')
	wait("演示配置")
	app.QueueUpdateDraw(func() { screen.SetSize(120, 30) })
	key(tcell.KeyRune, 'r')
	frame = wait("演示配置")
	for frame.Width != 120 {
		frame = wait("演示配置")
	}
	save("首页-120x30", frame)
}
