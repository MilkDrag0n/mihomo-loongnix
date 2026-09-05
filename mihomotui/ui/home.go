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
	summary, renderSummary := newHomeSummary()
	message := tview.NewTextView().SetDynamicColors(true)
	portInput := newInputField().SetLabel(" 端口: ").SetFieldWidth(5)
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
	applyPortButton := newActionButton(" 应用端口 ")
	coreButton := newActionButton(" 启动代理 ")
	tunButton := newActionButton(" 开启 TUN ")
	refreshButton := newActionButton(" 刷新 ")
	bar := tview.NewFlex().
		AddItem(portInput, 15, 0, false).
		AddItem(applyPortButton, 12, 0, false).
		AddItem(coreButton, 12, 0, true).
		AddItem(tunButton, 12, 0, false).
		AddItem(refreshButton, 8, 0, false)
	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(summary, 14, 0, false).AddItem(bar, 3, 0, true).
		AddItem(message, 1, 0, false).AddItem(nil, 0, 1, false)

	var mu sync.RWMutex
	var last mihomotui.ManagerStatus
	pending := false
	loaded := false
	render := func(status mihomotui.ManagerStatus) {
		renderSummary(status)
		if status.ProxyPort > 0 {
			portInput.SetText(strconv.Itoa(status.ProxyPort))
		} else {
			portInput.SetText("")
		}
		if status.Core.Running || status.Core.ServiceActive {
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

func newHomeSummary() (*tview.Flex, func(mihomotui.ManagerStatus)) {
	core := newStatusPanel("代理内核")
	tun := newStatusPanel("TUN 网络")
	connection := newStatusPanel("当前连接")
	core.SetText("[#9daaa6]正在读取内核状态…[-]")
	tun.SetText("[#9daaa6]正在读取网络状态…[-]")
	connection.SetText("[#9daaa6]正在读取配置与节点…[-]")
	top := tview.NewFlex().AddItem(core, 0, 1, false).AddItem(nil, 1, 0, false).AddItem(tun, 0, 1, false)
	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(top, 6, 0, false).AddItem(nil, 1, 0, false).
		AddItem(connection, 6, 0, false).AddItem(nil, 1, 0, false)
	return root, func(status mihomotui.ManagerStatus) {
		coreText, tunText, connectionText := homeStatusText(status)
		core.SetText(coreText)
		tun.SetText(tunText)
		connection.SetText(connectionText)
	}
}

func homeStatusText(status mihomotui.ManagerStatus) (string, string, string) {
	coreState := "[#e68a87::b]● 已停止[-::-]"
	if status.Core.Running {
		coreState = "[#a4d68a::b]● 运行中[-::-]"
	} else if status.Core.ServiceActive {
		coreState = "[#e6c384::b]● 服务活动，控制接口异常[-::-]"
	}
	tunState := "[#9daaa6::b]○ 已关闭[-::-]"
	if status.TUN.Enabled {
		tunState = "[#a4d68a::b]● 已开启[-::-]"
	} else if status.TUN.Configured {
		tunState = "[#e6c384::b]● 已预设，尚未生效[-::-]"
	}
	state := func(value bool, yes, no string) string {
		if value {
			return yes
		}
		return no
	}
	pid := "无"
	if status.Core.PID > 0 {
		pid = strconv.Itoa(status.Core.PID)
	}
	core := fmt.Sprintf("%s\n[#9daaa6]系统服务[-]  %s\n[#9daaa6]控制接口[-]  %s    [#9daaa6]进程[-] %s", coreState,
		state(status.Core.ServiceActive, "运行中", "未运行"), state(status.Core.ControllerHealthy, "可连接", "不可连接"), pid)
	tun := fmt.Sprintf("%s\n[#9daaa6]运行配置[-]  %s\n[#9daaa6]虚拟网卡[-]  %s", tunState,
		state(status.TUN.RuntimeEnabled, "已启用", "未启用"), state(status.TUN.InterfacePresent, "已就绪", "未创建"))
	profile := "尚未选择"
	if status.ActiveProfile != nil {
		profile = tview.Escape(status.ActiveProfile.Name)
	}
	port := "尚未设置"
	if status.ProxyPort > 0 {
		port = fmt.Sprintf("127.0.0.1:%d  HTTP/SOCKS", status.ProxyPort)
	}
	currentNode := "尚未选择"
	if status.CurrentNode != "" {
		currentNode = terminalNodeLabel(status.CurrentNode)
	}
	group := "无"
	if status.CurrentGroup != "" {
		group = tview.Escape(status.CurrentGroup)
	}
	connection := fmt.Sprintf("[#9daaa6]代理地址[-]  %s\n[#9daaa6]当前节点[-]  %s\n[#9daaa6]代理组  [-]  %s\n[#9daaa6]当前配置[-]  %s", port, currentNode, group, profile)
	return core, tun, connection
}

func formatHomeStatus(status mihomotui.ManagerStatus) string {
	core, tun, connection := homeStatusText(status)
	return core + "\n" + tun + "\n" + connection
}
