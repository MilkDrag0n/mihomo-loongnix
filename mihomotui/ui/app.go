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
	configureNeutralTheme()
	if err := mihomotui.IPCProbeDaemon(); err != nil {
		return fmt.Errorf("无法连接 mihomo-manager: %w", err)
	}
	client, err := mihomotui.GetIPCClient()
	if err != nil {
		return err
	}
	app := tview.NewApplication().EnableMouse(true).EnablePaste(true)
	overlay := tview.NewPages()
	overlay.Focus(func(p tview.Primitive) { app.SetFocus(p) })

	views := map[string]*pageView{}
	views["home"] = newHomePage(app, client, overlay)
	views["profiles"] = newProfilesPage(app, client, overlay)
	views["nodes"] = newNodesPage(app, client, overlay)
	views["rules"] = newRulesPage(app, client, overlay)
	views["logs"] = newLogsPage(app, client, overlay)

	pages := tview.NewPages()
	pages.Focus(func(p tview.Primitive) { app.SetFocus(p) })
	for _, id := range []string{"home", "profiles", "nodes", "rules", "logs"} {
		pages.AddPage(id, views[id], true, id == "home")
	}
	current := "home"
	var nav *tview.List
	switchPage := func(id string) {
		if id == current {
			views[id].Refresh()
			if views[id].first != nil {
				app.SetFocus(views[id].first)
			}
			return
		}
		views[current].Stop()
		current = id
		pages.SwitchToPage(id)
		views[id].Start()
		if views[id].first != nil {
			app.SetFocus(views[id].first)
		}
	}
	nav = tview.NewList().ShowSecondaryText(false)
	for _, item := range []struct{ id, label string }{{"home", "1 首页"}, {"profiles", "2 配置"}, {"nodes", "3 节点"}, {"rules", "4 规则"}, {"logs", "5 日志"}} {
		id := item.id
		nav.AddItem(item.label, "", 0, func() { switchPage(id) })
	}
	nav.SetBorder(true).SetTitle(" 菜单 ")
	main := tview.NewFlex().AddItem(nav, 16, 0, true).AddItem(pages, 0, 1, false)
	overlay.AddPage("main", main, true, true)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		focus := app.GetFocus()
		if event.Key() == tcell.KeyRune && !isTextInput(focus) {
			switch event.Rune() {
			case '1', '2', '3', '4', '5':
				ids := []string{"home", "profiles", "nodes", "rules", "logs"}
				switchPage(ids[int(event.Rune()-'1')])
				return nil
			case 'q':
				views[current].Stop()
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
		if views[current].pageKey != nil && views[current].pageKey(event) {
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
			if isTextInput(focus) {
				if views[current].first != nil {
					app.SetFocus(views[current].first)
				}
			} else {
				app.SetFocus(nav)
			}
			return nil
		}
		return event
	})
	views[current].Start()
	err = app.SetRoot(overlay, true).SetFocus(nav).Run()
	views[current].Stop()
	return err
}

func reportError(app *tview.Application, pages *tview.Pages, title string, err error) {
	app.QueueUpdateDraw(func() { ShowAlertModal(app, pages, title, mihomotui.RedactURLInText(err.Error())) })
}
