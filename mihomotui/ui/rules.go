package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"mihomotui/mihomotui"
)

func filterRules(rules []mihomotui.Rule, keyword string) []mihomotui.Rule {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return append([]mihomotui.Rule(nil), rules...)
	}
	result := make([]mihomotui.Rule, 0, len(rules))
	for _, rule := range rules {
		if strings.Contains(strings.ToLower(rule.Content), keyword) || strings.Contains(strings.ToLower(rule.Type), keyword) || strings.Contains(strings.ToLower(rule.Policy), keyword) {
			result = append(result, rule)
		}
	}
	return result
}

func newRulesPage(app *tview.Application, client *mihomotui.IPCClient, overlay *tview.Pages) *pageView {
	filterInput := newInputField().SetLabel(" 筛选: ").SetPlaceholder("规则、类型或策略")
	refreshButton := newActionButton(" 刷新 ")
	toolbar := tview.NewFlex().AddItem(filterInput, 0, 1, true).AddItem(refreshButton, 10, 0, false)
	table := tview.NewTable().SetSelectable(true, false).SetFixed(1, 0)
	table.SetBorder(true).SetTitle(" 当前生效规则（只读） ")
	styleTable(table)
	prevButton := newActionButton(" 上一页 ")
	nextButton := newActionButton(" 下一页 ")
	pageInfo := tview.NewTextView().SetTextAlign(tview.AlignCenter)
	bottom := tview.NewFlex().AddItem(prevButton, 10, 0, true).AddItem(pageInfo, 24, 0, false).AddItem(nextButton, 10, 0, false)
	root := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(toolbar, 3, 0, true).AddItem(table, 0, 1, false).AddItem(bottom, 3, 0, false)
	var mu sync.Mutex
	var all []mihomotui.Rule
	pages := pager{PageSize: 15}
	render := func() {
		mu.Lock()
		defer mu.Unlock()
		filtered := filterRules(all, filterInput.GetText())
		pages.Total = len(filtered)
		start, end := pages.Bounds()
		table.Clear()
		for col, heading := range []string{"#", "规则", "类型", "策略"} {
			table.SetCell(0, col, tview.NewTableCell(heading).SetSelectable(false).SetTextColor(colorAccent))
		}
		for i := start; i < end; i++ {
			rule, row := filtered[i], i-start+1
			table.SetCell(row, 0, tview.NewTableCell(fmt.Sprint(i+1)))
			table.SetCell(row, 1, tview.NewTableCell(rule.Content))
			table.SetCell(row, 2, tview.NewTableCell(rule.Type))
			table.SetCell(row, 3, tview.NewTableCell(rule.Policy))
		}
		if len(filtered) == 0 {
			table.SetCell(1, 1, tview.NewTableCell("无匹配规则").SetTextColor(colorMuted))
		}
		pageInfo.SetText(fmt.Sprintf("%d / %d  ·  %d / %d", pages.Page+1, pages.Pages(), len(filtered), len(all)))
	}
	refresh := func() {
		go func() {
			rules, err := client.ManagerRules()
			if err != nil {
				reportError(app, overlay, "读取规则失败", err)
				return
			}
			mu.Lock()
			all = rules
			pages.Page = 0
			mu.Unlock()
			app.QueueUpdateDraw(render)
		}()
	}
	filterInput.SetChangedFunc(func(string) { mu.Lock(); pages.Page = 0; mu.Unlock(); render() })
	refreshButton.SetSelectedFunc(refresh)
	prev := func() { mu.Lock(); pages.Prev(); mu.Unlock(); render() }
	next := func() { mu.Lock(); pages.Next(); mu.Unlock(); render() }
	prevButton.SetSelectedFunc(prev)
	nextButton.SetSelectedFunc(next)
	view := &pageView{Primitive: root, focusables: []tview.Primitive{filterInput, refreshButton, table, prevButton, nextButton}, first: table, filter: filterInput, refresh: refresh}
	view.pageKey = func(event *tcell.EventKey) bool {
		switch event.Key() {
		case tcell.KeyPgUp:
			prev()
			return true
		case tcell.KeyPgDn:
			next()
			return true
		case tcell.KeyHome:
			mu.Lock()
			pages.First()
			mu.Unlock()
			render()
			return true
		case tcell.KeyEnd:
			mu.Lock()
			pages.Last()
			mu.Unlock()
			render()
			return true
		}
		return false
	}
	return view
}
