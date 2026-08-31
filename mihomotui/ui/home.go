package ui

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"mihomotui/mihomotui"
)

func newHomePage(app *tview.Application, client *mihomotui.IPCClient, overlay *tview.Pages) *pageView {
	statusView := tview.NewTextView().SetDynamicColors(true).SetWrap(true)
	statusView.SetBorder(true).SetTitle(" 实际运行状态 ")
	message := tview.NewTextView().SetDynamicColors(true)
	portInput := tview.NewInputField().SetLabel(" 端口: ").SetFieldWidth(5)
	portInput.SetAcceptanceFunc(func(text string, _ rune) bool {
		if text == "" {
			return true
		}
		if len(text) > 5 {
			return false
		}
		_, err := strconv.ParseUint(text, 10, 16)
		return err == nil
	})
	applyPortButton := tview.NewButton(" 应用端口 ")
	coreButton := tview.NewButton(" 启动代理 ")
	tunButton := tview.NewButton(" 开启 TUN ")
	refreshButton := tview.NewButton(" 刷新 ")
	bar := tview.NewFlex().
		AddItem(portInput, 15, 0, false).
		AddItem(applyPortButton, 12, 0, false).
		AddItem(coreButton, 12, 0, true).
		AddItem(tunButton, 12, 0, false).
		AddItem(refreshButton, 8, 0, false)
	root := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(statusView, 0, 1, false).AddItem(message, 2, 0, false).AddItem(bar, 3, 0, true)

	var mu sync.RWMutex
	var last mihomotui.ManagerStatus
	pending := false
	loaded := false
	render := func(status mihomotui.ManagerStatus) {
		statusView.SetText(formatHomeStatus(status))
		if status.ProxyPort > 0 {
			portInput.SetText(strconv.Itoa(status.ProxyPort))
		} else {
			portInput.SetText("")
		}
		if status.Core.Running {
			coreButton.SetLabel(" 停止代理 ")
		} else {
			coreButton.SetLabel(" 启动代理 ")
		}
		if status.TUN.Configured {
			tunButton.SetLabel(" 关闭 TUN ")
		} else {
			tunButton.SetLabel(" 开启 TUN ")
		}
	}
	refresh := func() {
		go func() {
			status, err := client.ManagerStatus()
			if err != nil {
				reportError(app, overlay, "刷新失败", err)
				return
			}
			mu.Lock()
			last = *status
			loaded = true
			mu.Unlock()
			app.QueueUpdateDraw(func() { render(*status); message.SetText("") })
		}()
	}
	run := func(label string, action func(mihomotui.ManagerStatus) (*mihomotui.ManagerStatus, error)) {
		mu.Lock()
		if pending {
			mu.Unlock()
			return
		}
		if !loaded {
			mu.Unlock()
			message.SetText("[yellow]尚未取得后端状态，正在刷新…[-]")
			refresh()
			return
		}
		pending = true
		snapshot := last
		mu.Unlock()
		message.SetText("[yellow]" + label + "，正在等待后端状态回读…[-]")
		go func() {
			result, err := action(snapshot)
			mu.Lock()
			pending = false
			if result != nil {
				last = *result
			}
			mu.Unlock()
			if err != nil {
				reportError(app, overlay, label+"失败", err)
				app.QueueUpdateDraw(func() { message.SetText("[red]操作失败，界面保持后端实际状态[-]") })
				refresh()
				return
			}
			app.QueueUpdateDraw(func() { render(*result); message.SetText("[green]操作成功，已通过后端状态回读确认[-]") })
		}()
	}
	applyPort := func() {
		port, err := strconv.Atoi(portInput.GetText())
		if err != nil || port < 1 || port > 65535 {
			ShowAlertModal(app, overlay, "端口无效", "代理端口必须在 1-65535 之间")
			return
		}
		run("应用代理端口", func(mihomotui.ManagerStatus) (*mihomotui.ManagerStatus, error) {
			return client.ManagerSetProxyPort(port)
		})
	}
	portInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			applyPort()
		}
	})
	applyPortButton.SetSelectedFunc(applyPort)
	coreButton.SetSelectedFunc(func() {
		run("切换代理", func(s mihomotui.ManagerStatus) (*mihomotui.ManagerStatus, error) {
			if s.Core.Running || s.Core.ServiceActive {
				return client.ManagerStopCore()
			}
			return client.ManagerStartCore()
		})
	})
	tunButton.SetSelectedFunc(func() {
		run("切换 TUN", func(s mihomotui.ManagerStatus) (*mihomotui.ManagerStatus, error) {
			return client.ManagerSetTUN(!s.TUN.Configured)
		})
	})
	refreshButton.SetSelectedFunc(refresh)
	return &pageView{Primitive: root, focusables: []tview.Primitive{portInput, applyPortButton, coreButton, tunButton, refreshButton}, first: coreButton, refresh: refresh}
}

func formatHomeStatus(status mihomotui.ManagerStatus) string {
	coreState := "[red]已停止[-]"
	if status.Core.Running {
		coreState = "[green]运行中[-]"
	} else if status.Core.ServiceActive {
		coreState = "[yellow]服务活动但控制接口异常[-]"
	}
	tunState := "[gray]关闭[-]"
	if status.TUN.Enabled {
		tunState = "[green]已开启[-]"
	} else if status.TUN.Configured {
		tunState = "[yellow]已预设，尚未实际生效[-]"
	}
	profile := "无"
	if status.ActiveProfile != nil {
		profile = tview.Escape(status.ActiveProfile.Name)
	}
	port := "未监听"
	if status.ProxyPort > 0 {
		port = fmt.Sprintf("127.0.0.1:%d（HTTP/SOCKS）", status.ProxyPort)
	}
	currentNode := "无"
	if status.CurrentNode != "" {
		currentNode = terminalNodeLabel(status.CurrentNode)
		if status.CurrentGroup != "" {
			currentNode += "  （" + tview.Escape(status.CurrentGroup) + "）"
		}
	}
	return fmt.Sprintf("\n  代理内核: %s\n  systemd: %t   控制接口: %t   PID: %d\n\n  代理端口: %s\n  当前节点: %s\n\n  TUN: %s\n  运行配置: %t   虚拟网卡: %t\n\n  当前配置: %s\n", coreState, status.Core.ServiceActive, status.Core.ControllerHealthy, status.Core.PID, port, currentNode, tunState, status.TUN.RuntimeEnabled, status.TUN.InterfacePresent, profile)
}
