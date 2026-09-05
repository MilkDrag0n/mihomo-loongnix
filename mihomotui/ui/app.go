package ui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"mihomotui/mihomotui"
)

// Run starts the unprivileged client. The standalone flag is retained only for
// CLI compatibility; privileged actions always require the manager service.
func Run(_ bool) error {
	if err := mihomotui.IPCProbeDaemon(); err != nil {
		return fmt.Errorf("无法连接 mihomo-manager: %w", err)
	}
	client, err := mihomotui.GetIPCClient()
	if err != nil {
		return err
	}
	app, _, start, stop := newClientApplication(client)
	start()
	defer stop()
	return app.Run()
}

// 单独构建界面，允许使用模拟终端验证布局与键盘行为。
func newClientApplication(client *mihomotui.IPCClient) (*tview.Application, *tview.Pages, func(), func()) {
	configureNeutralTheme()
	app := tview.NewApplication().EnableMouse(true).EnablePaste(true)
	overlay := tview.NewPages()
	overlay.Focus(func(p tview.Primitive) { app.SetFocus(p) })
	views := map[string]*pageView{
		"home":     newHomePage(app, client, overlay),
		"profiles": newProfilesPage(app, client, overlay),
		"nodes":    newNodesPage(app, client, overlay),
		"rules":    newRulesPage(app, client, overlay),
		"logs":     newLogsPage(app, client, overlay),
	}
	ids := []string{"home", "profiles", "nodes", "rules", "logs"}
	labels := []string{"首页", "配置", "节点", "规则", "日志"}
	pages := tview.NewPages()
	pages.Focus(func(p tview.Primitive) { app.SetFocus(p) })
	for _, id := range ids {
		pages.AddPage(id, views[id], true, id == "home")
	}
	current := "home"
	nav := tview.NewTable().SetSelectable(false, true)
	nav.SetBorder(true)
	nav.SetSelectedStyle(tcell.StyleDefault.Reverse(true).Bold(true))
	focusBorder(nav.Box)
	header := tview.NewTextView().SetDynamicColors(true)
	header.SetText(" [#8cd5bd::b]MIHOMO[-::-]  代理控制台")
	footer := tview.NewTextView().SetDynamicColors(true).SetTextColor(colorMuted)
	updateNavigation := func() {
		for i, id := range ids {
			cell := tview.NewTableCell(fmt.Sprintf(" %d %s ", i+1, labels[i])).SetAlign(tview.AlignCenter).SetExpansion(1).SetTextColor(colorMuted)
			if id == current {
				cell.SetTextColor(colorAccent).SetAttributes(tcell.AttrBold)
				nav.Select(0, i)
			}
			nav.SetCell(0, i, cell)
		}
		hint := " [#8cd5bd]1–5[-] 切页  [#8cd5bd]Tab[-] 移动  [#8cd5bd]Enter[-] 确认  [#8cd5bd]r[-] 刷新  [#8cd5bd]Esc[-] 导航  [#8cd5bd]q[-] 退出"
		if views[current].filter != nil {
			hint += "  [#8cd5bd]/[-] 筛选"
		}
		footer.SetText(hint)
	}
	switchPage := func(id string) {
		if id != current {
			views[current].Stop()
			current = id
			pages.SwitchToPage(id)
			views[id].Start()
		} else {
			views[id].Refresh()
		}
		updateNavigation()
		if views[id].first != nil {
			app.SetFocus(views[id].first)
		}
	}
	updateNavigation()
	nav.SetSelectedFunc(func(_, column int) { switchPage(ids[column]) })
	main := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 1, 0, false).
		AddItem(nav, 3, 0, true).
		AddItem(nil, 1, 0, false).
		AddItem(pages, 0, 1, false).
		AddItem(footer, 1, 0, false)
	overlay.AddPage("main", main, true, true)
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// 弹窗和展开的下拉框自己处理按键，禁止全局快捷键穿透。
		if name, _ := overlay.GetFrontPage(); name != "main" {
			return event
		}
		focus := app.GetFocus()
		for _, primitive := range views[current].focusables {
			if drop, ok := primitive.(*tview.DropDown); ok && drop.HasFocus() && drop.IsOpen() {
				return event
			}
		}
		if event.Key() == tcell.KeyRune && !isTextInput(focus) {
			switch event.Rune() {
			case '1', '2', '3', '4', '5':
				switchPage(ids[int(event.Rune()-'1')])
				return nil
			case 'q':
				app.Stop()
				return nil
			case 'r':
				views[current].Refresh()
				return nil
			case '/':
				if views[current].filter != nil {
					app.SetFocus(views[current].filter)
					return nil
				}
			case 'j':
				return tcell.NewEventKey(tcell.KeyDown, 0, event.Modifiers())
			case 'k':
				return tcell.NewEventKey(tcell.KeyUp, 0, event.Modifiers())
			}
		}
		if !isTextInput(focus) && views[current].pageKey != nil && views[current].pageKey(event) {
			return nil
		}
		switch event.Key() {
		case tcell.KeyTab:
			views[current].focusNext(app, focus, false)
			return nil
		case tcell.KeyBacktab:
			views[current].focusNext(app, focus, true)
			return nil
		case tcell.KeyEsc:
			if isTextInput(focus) && views[current].first != nil {
				app.SetFocus(views[current].first)
			} else {
				app.SetFocus(nav)
			}
			return nil
		}
		return event
	})
	app.SetRoot(overlay, true).SetFocus(nav)
	return app, overlay, func() { views[current].Start() }, func() { views[current].Stop() }
}

func reportError(app *tview.Application, pages *tview.Pages, title string, err error) {
	app.QueueUpdateDraw(func() { ShowAlertModal(app, pages, title, mihomotui.RedactURLInText(err.Error())) })
}
