package ui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rivo/tview"
	"mihomotui/mihomotui"
)

type logEntry struct{ Time, Level, Message string }

const maxVisibleLogs = 2000

func formatBytes(value int64) string {
	units := []string{"B", "KiB", "MiB", "GiB"}
	size := float64(value)
	unit := 0
	for size >= 1024 && unit < len(units)-1 {
		size /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", value, units[unit])
	}
	return fmt.Sprintf("%.2f %s", size, units[unit])
}

func newLogsPage(app *tview.Application, client *mihomotui.IPCClient, overlay *tview.Pages) *pageView {
	statusView := tview.NewTextView().SetDynamicColors(true).SetWrap(true)
	filterInput := newInputField().SetLabel(" 筛选: ").SetPlaceholder("日志内容")
	levelDrop := newDropDown().SetLabel(" 级别: ").SetOptions([]string{"全部", "DEBUG", "INFO", "WARNING", "ERROR"}, nil)
	recordButton := newActionButton(" 开启磁盘记录 ")
	pauseButton := newActionButton(" 暂停显示 ")
	clearButton := newActionButton(" 清空界面 ")
	refreshButton := newActionButton(" 刷新大小 ")
	toolbar1 := tview.NewFlex().AddItem(filterInput, 0, 2, true).AddItem(levelDrop, 20, 0, false).AddItem(recordButton, 18, 0, false)
	toolbar2 := tview.NewFlex().AddItem(pauseButton, 14, 0, true).AddItem(clearButton, 14, 0, false).AddItem(refreshButton, 14, 0, false).AddItem(nil, 0, 1, false)
	logView := tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetWrap(true)
	logView.SetBorder(true).SetTitle(" Mihomo 实时日志 ")
	focusBorder(logView.Box)
	// Keep authoritative disk state on its own two-line row. At the common
	// 80-column terminal width it otherwise competes with the action buttons
	// and hides total size or the rotation policy.
	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(toolbar1, 3, 0, true).
		AddItem(toolbar2, 3, 0, false).
		AddItem(statusView, 2, 0, false).
		AddItem(logView, 0, 1, false)
	var mu sync.Mutex
	var entries []logEntry
	level := ""
	paused := false
	pending := false
	var disk mihomotui.LoggingStatus
	var cancel context.CancelFunc
	renderStatus := func() {
		mu.Lock()
		snapshot := disk
		mu.Unlock()
		state := "[gray]关闭[-]"
		label := "最近文件"
		if snapshot.Enabled {
			state = "[green]开启[-]"
			label = "当前文件"
			recordButton.SetLabel(" 关闭磁盘记录 ")
		} else {
			recordButton.SetLabel(" 开启磁盘记录 ")
		}
		name := snapshot.CurrentFile
		if name == "" {
			name = "无"
		}
		statusView.SetText(fmt.Sprintf(" 记录: %s  %s: %s (%s)  总量: %s  轮转: %s × %d ", state, label, tview.Escape(name), formatBytes(snapshot.CurrentFileBytes), formatBytes(snapshot.TotalBytes), formatBytes(snapshot.MaxFileBytes), snapshot.MaxBackups))
	}
	renderLogs := func() {
		mu.Lock()
		defer mu.Unlock()
		logView.Clear()
		keyword := strings.ToLower(strings.TrimSpace(filterInput.GetText()))
		var b strings.Builder
		for _, entry := range entries {
			if level != "" && entry.Level != level {
				continue
			}
			if keyword != "" && !strings.Contains(strings.ToLower(entry.Message), keyword) {
				continue
			}
			color := "white"
			switch entry.Level {
			case "DEBUG":
				color = "gray"
			case "INFO":
				color = "green"
			case "WARNING":
				color = "yellow"
			case "ERROR":
				color = "red"
			}
			fmt.Fprintf(&b, "[%s]%s [%-7s] %s[-]\n", color, entry.Time, entry.Level, tview.Escape(entry.Message))
		}
		fmt.Fprint(logView, b.String())
		logView.ScrollToEnd()
	}
	refreshStatus := func() {
		go func() {
			status, err := client.ManagerLoggingStatus()
			if err != nil {
				reportError(app, overlay, "读取日志状态失败", err)
				return
			}
			mu.Lock()
			disk = *status
			mu.Unlock()
			app.QueueUpdateDraw(renderStatus)
		}()
	}
	appendEntry := func(entry logEntry) {
		mu.Lock()
		if len(entries) >= maxVisibleLogs {
			copy(entries, entries[len(entries)-maxVisibleLogs+1:])
			entries = entries[:maxVisibleLogs-1]
		}
		entries = append(entries, entry)
		shouldRender := !paused
		mu.Unlock()
		if shouldRender {
			renderLogs()
		}
	}
	stream := func(ctx context.Context) {
		// Subscribe at debug level once; the level dropdown is a stable
		// client-side filter and must not silently miss lower-severity events.
		resp, err := client.ManagerLogStream("debug")
		if err != nil {
			if ctx.Err() == nil {
				reportError(app, overlay, "实时日志连接失败", err)
			}
			return
		}
		defer resp.Body.Close()
		go func() { <-ctx.Done(); _ = resp.Body.Close() }()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			if ctx.Err() != nil {
				return
			}
			line := strings.TrimSpace(scanner.Text())
			line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if line == "" {
				continue
			}
			var message struct {
				Type    string `json:"type"`
				Payload string `json:"payload"`
			}
			if json.Unmarshal([]byte(line), &message) != nil {
				continue
			}
			levelName := strings.ToUpper(message.Type)
			if levelName == "WARN" {
				levelName = "WARNING"
			}
			if levelName == "" {
				levelName = "INFO"
			}
			entry := logEntry{Time: time.Now().Format("15:04:05"), Level: levelName, Message: message.Payload}
			app.QueueUpdateDraw(func() { appendEntry(entry) })
		}
	}
	start := func() {
		if cancel != nil {
			cancel()
		}
		ctx, c := context.WithCancel(context.Background())
		cancel = c
		go stream(ctx)
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					status, err := client.ManagerLoggingStatus()
					if err == nil {
						mu.Lock()
						disk = *status
						mu.Unlock()
						app.QueueUpdateDraw(renderStatus)
					}
				}
			}
		}()
	}
	stop := func() {
		if cancel != nil {
			cancel()
			cancel = nil
		}
	}
	filterInput.SetChangedFunc(func(string) { renderLogs() })
	levelDrop.SetSelectedFunc(func(text string, index int) {
		mu.Lock()
		if index == 0 {
			level = ""
		} else {
			level = text
		}
		mu.Unlock()
		renderLogs()
	})
	recordButton.SetSelectedFunc(func() {
		mu.Lock()
		if pending {
			mu.Unlock()
			return
		}
		pending = true
		target := !disk.Enabled
		mu.Unlock()
		go func() {
			status, err := client.ManagerSetLogging(target)
			mu.Lock()
			pending = false
			if err == nil {
				disk = *status
			}
			mu.Unlock()
			if err != nil {
				reportError(app, overlay, "切换磁盘日志失败", err)
				return
			}
			app.QueueUpdateDraw(renderStatus)
		}()
	})
	pauseButton.SetSelectedFunc(func() {
		mu.Lock()
		paused = !paused
		nowPaused := paused
		mu.Unlock()
		if nowPaused {
			pauseButton.SetLabel(" 继续显示 ")
		} else {
			pauseButton.SetLabel(" 暂停显示 ")
			renderLogs()
		}
	})
	clearButton.SetSelectedFunc(func() { mu.Lock(); entries = nil; mu.Unlock(); logView.Clear() })
	refreshButton.SetSelectedFunc(refreshStatus)
	return &pageView{Primitive: root, focusables: []tview.Primitive{filterInput, levelDrop, recordButton, pauseButton, clearButton, refreshButton, logView}, first: logView, filter: filterInput, start: start, stop: stop, refresh: refreshStatus}
}
