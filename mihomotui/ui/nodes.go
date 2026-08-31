package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"mihomotui/mihomotui"
)

func filterNodes(nodes []mihomotui.ProxyNode, keyword string) []mihomotui.ProxyNode {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return append([]mihomotui.ProxyNode(nil), nodes...)
	}
	result := make([]mihomotui.ProxyNode, 0, len(nodes))
	for _, node := range nodes {
		if strings.Contains(strings.ToLower(node.Name), keyword) || strings.Contains(strings.ToLower(node.Type), keyword) {
			result = append(result, node)
		}
	}
	return result
}

func nodeTestKey(group, node string) string { return group + "\x00" + node }

func newNodesPage(app *tview.Application, client *mihomotui.IPCClient, overlay *tview.Pages) *pageView {
	groupDrop := tview.NewDropDown().SetLabel(" 代理组: ")
	filterInput := tview.NewInputField().SetLabel(" 筛选: ").SetPlaceholder("节点名称或类型")
	refreshButton := tview.NewButton(" 刷新 ")
	toolbar := tview.NewFlex().AddItem(groupDrop, 0, 1, true).AddItem(filterInput, 0, 2, false).AddItem(refreshButton, 10, 0, false)
	table := tview.NewTable().SetSelectable(true, true).SetFixed(1, 0)
	table.SetBorder(true).SetTitle(" 节点 ")
	prevButton := tview.NewButton(" 上一页 ")
	nextButton := tview.NewButton(" 下一页 ")
	selectButton := tview.NewButton(" 使用所选节点 ")
	pageInfo := tview.NewTextView().SetTextAlign(tview.AlignCenter).SetDynamicColors(true)
	bottom := tview.NewFlex().AddItem(prevButton, 10, 0, true).AddItem(pageInfo, 18, 0, false).AddItem(nextButton, 10, 0, false).AddItem(selectButton, 18, 0, false)
	message := tview.NewTextView().SetDynamicColors(true)
	root := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(toolbar, 3, 0, true).AddItem(table, 0, 1, false).AddItem(message, 2, 0, false).AddItem(bottom, 3, 0, false)

	var groups []mihomotui.ProxyGroup
	groupIndex := 0
	var visible []mihomotui.ProxyNode
	selectedNode := ""
	pages := pager{PageSize: 12}
	pendingSelection := false
	testingNodes := make(map[string]bool)
	var testNode func(group, node string)
	const (
		nodeRegionColumn = 1
		nodeNameColumn   = 2
		nodeSpeedColumn  = 5
	)

	markNodeDelay := func(groupName, nodeName string, delay int) bool {
		for groupPos := range groups {
			if groups[groupPos].Name != groupName {
				continue
			}
			for nodePos := range groups[groupPos].Nodes {
				if groups[groupPos].Nodes[nodePos].Name == nodeName {
					groups[groupPos].Nodes[nodePos].Delay = delay
					return true
				}
			}
		}
		return false
	}

	render := func() {
		_, selectedColumn := table.GetSelection()
		if selectedColumn < 0 || selectedColumn > nodeSpeedColumn {
			selectedColumn = 0
		}
		table.Clear()
		for col, heading := range []string{"状态", "地区", "节点", "类型", "延迟", "测速"} {
			table.SetCell(0, col, tview.NewTableCell(heading).SetTextColor(tcell.ColorYellow).SetSelectable(false))
		}
		if groupIndex >= len(groups) {
			pageInfo.SetText("0 / 0")
			return
		}
		group := groups[groupIndex]
		visible = filterNodes(group.Nodes, filterInput.GetText())
		pages.Total = len(visible)
		start, end := pages.Bounds()
		for i := start; i < end; i++ {
			node, row := visible[i], i-start+1
			state, color := "", tcell.ColorWhite
			if node.Name == group.Now {
				state, color = "● 当前", tcell.ColorGreen
			}
			region, label := terminalNodeColumns(node.Name)
			table.SetCell(row, 0, tview.NewTableCell(state).SetTextColor(color).SetMaxWidth(7))
			table.SetCell(row, nodeRegionColumn, tview.NewTableCell(tview.Escape(region)).SetReference(node.Name).SetTextColor(color).SetMaxWidth(4))
			table.SetCell(row, nodeNameColumn, tview.NewTableCell(tview.Escape(label)).SetReference(node.Name).SetTextColor(color).SetExpansion(1))
			table.SetCell(row, 3, tview.NewTableCell(node.Type).SetMaxWidth(14))
			table.SetCell(row, 4, tview.NewTableCell(DelayText(node.Delay)).SetTextColor(tcell.GetColor(DelayColor(node.Delay))).SetAlign(tview.AlignRight).SetMaxWidth(8))

			groupName, nodeName := group.Name, node.Name
			buttonText := " 测速 "
			buttonColor := tcell.NewHexColor(0x303030)
			if testingNodes[nodeTestKey(groupName, nodeName)] {
				buttonText = " 测试中 "
				buttonColor = tcell.NewHexColor(0x505050)
			}
			button := tview.NewTableCell(buttonText).
				SetAlign(tview.AlignCenter).
				SetTextColor(tcell.ColorWhite).
				SetBackgroundColor(buttonColor).
				SetMaxWidth(8)
			button.SetClickedFunc(func() bool {
				selectedNode = nodeName
				table.Select(row, nodeSpeedColumn)
				testNode(groupName, nodeName)
				return true
			})
			table.SetCell(row, nodeSpeedColumn, button)
			if node.Name == selectedNode {
				table.Select(row, selectedColumn)
			}
		}
		if len(visible) == 0 {
			table.SetCell(1, nodeNameColumn, tview.NewTableCell("无匹配节点").SetTextColor(tcell.ColorGray))
		}
		pageInfo.SetText(fmt.Sprintf("%d / %d  ·  %d 节点", pages.Page+1, pages.Pages(), len(visible)))
	}

	load := func() {
		go func() {
			items, err := client.ManagerProxyGroups()
			if err != nil {
				reportError(app, overlay, "读取节点失败", err)
				return
			}
			names := make([]string, len(items))
			for i, group := range items {
				names[i] = group.Name
			}
			app.QueueUpdateDraw(func() {
				oldName := ""
				if groupIndex < len(groups) {
					oldName = groups[groupIndex].Name
				}
				groups = items
				groupIndex = 0
				for i, group := range items {
					if group.Name == oldName {
						groupIndex = i
					}
				}
				if len(items) > 0 {
					selectedNode = items[groupIndex].Now
				} else {
					selectedNode = ""
				}
				pages.Page = 0
				groupDrop.SetOptions(names, nil)
				if len(names) > 0 {
					groupDrop.SetCurrentOption(groupIndex)
				}
				render()
				message.SetText("")
			})
		}()
	}

	groupDrop.SetSelectedFunc(func(text string, index int) {
		groupIndex = index
		pages.Page = 0
		if index < len(groups) {
			selectedNode = groups[index].Now
		}
		render()
	})
	filterInput.SetChangedFunc(func(string) { pages.Page = 0; render() })
	table.SetSelectionChangedFunc(func(row, col int) {
		if row < 1 {
			return
		}
		if value, ok := table.GetCell(row, nodeNameColumn).GetReference().(string); ok {
			selectedNode = value
		}
	})

	selectCurrent := func() {
		if pendingSelection || groupIndex >= len(groups) || selectedNode == "" {
			return
		}
		pendingSelection = true
		groupName, node := groups[groupIndex].Name, selectedNode
		message.SetText("[yellow]正在切换并等待内核回读…[-]")
		go func() {
			group, err := client.ManagerSelectProxy(groupName, node)
			if err != nil {
				reportError(app, overlay, "切换节点失败", err)
				app.QueueUpdateDraw(func() {
					pendingSelection = false
					message.SetText("[red]切换失败，选中状态未改变[-]")
				})
				return
			}
			app.QueueUpdateDraw(func() {
				pendingSelection = false
				if groupIndex < len(groups) && groups[groupIndex].Name == group.Name {
					groups[groupIndex] = *group
					selectedNode = group.Now
				}
				render()
				message.SetText("[green]已通过内核 now 字段确认切换成功[-]")
			})
		}()
	}

	testNode = func(groupName, nodeName string) {
		key := nodeTestKey(groupName, nodeName)
		if testingNodes[key] {
			return
		}
		previousDelay := mihomotui.DelayUntested
		for _, group := range groups {
			if group.Name != groupName {
				continue
			}
			for _, node := range group.Nodes {
				if node.Name == nodeName {
					previousDelay = node.Delay
				}
			}
		}
		testingNodes[key] = true
		markNodeDelay(groupName, nodeName, mihomotui.DelayTesting)
		message.SetText(fmt.Sprintf("[yellow]正在测试 %s 的实际延迟…[-]", terminalNodeLabel(nodeName)))
		render()
		go func() {
			result, err := client.ManagerTestProxyDelay(groupName, nodeName)
			if err != nil {
				reportError(app, overlay, "节点测速失败", err)
				app.QueueUpdateDraw(func() {
					delete(testingNodes, key)
					markNodeDelay(groupName, nodeName, previousDelay)
					render()
					message.SetText("[red]测速失败，已恢复原延迟显示[-]")
				})
				return
			}
			app.QueueUpdateDraw(func() {
				delete(testingNodes, key)
				markNodeDelay(groupName, nodeName, result.Delay)
				render()
				message.SetText(fmt.Sprintf("[green]%s 实测延迟：%s[-]", terminalNodeLabel(nodeName), DelayText(result.Delay)))
			})
		}()
	}

	table.SetSelectedFunc(func(row, col int) {
		if row < 1 {
			return
		}
		if col == nodeSpeedColumn {
			if node, ok := table.GetCell(row, nodeNameColumn).GetReference().(string); ok && groupIndex < len(groups) {
				testNode(groups[groupIndex].Name, node)
			}
			return
		}
		selectCurrent()
	})
	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune && event.Rune() == ' ' {
			return tcell.NewEventKey(tcell.KeyEnter, 0, event.Modifiers())
		}
		return event
	})
	selectButton.SetSelectedFunc(selectCurrent)
	prev := func() { pages.Prev(); render() }
	next := func() { pages.Next(); render() }
	prevButton.SetSelectedFunc(prev)
	nextButton.SetSelectedFunc(next)
	refreshButton.SetSelectedFunc(load)
	view := &pageView{Primitive: root, focusables: []tview.Primitive{groupDrop, filterInput, refreshButton, table, prevButton, nextButton, selectButton}, first: table, filter: filterInput, refresh: load}
	view.pageKey = func(event *tcell.EventKey) bool {
		switch event.Key() {
		case tcell.KeyPgUp:
			prev()
			return true
		case tcell.KeyPgDn:
			next()
			return true
		case tcell.KeyHome:
			pages.First()
			render()
			return true
		case tcell.KeyEnd:
			pages.Last()
			render()
			return true
		}
		return false
	}
	return view
}
