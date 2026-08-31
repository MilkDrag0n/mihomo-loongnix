package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"mihomotui/mihomotui"
)

func newProfilesPage(app *tview.Application, client *mihomotui.IPCClient, overlay *tview.Pages) *pageView {
	nameInput := tview.NewInputField().SetLabel(" 名称: ").SetPlaceholder("可选；重命名时填写新名称")
	urlInput := tview.NewInputField().SetLabel(" 链接: ").SetPlaceholder("https://...")
	importButton := tview.NewButton(" 导入 ")
	inputs := tview.NewFlex().AddItem(nameInput, 0, 1, true).AddItem(urlInput, 0, 2, false).AddItem(importButton, 10, 0, false)
	table := tview.NewTable().SetSelectable(true, false).SetFixed(1, 0)
	table.SetBorder(true).SetTitle(" 配置列表 ")
	activateButton := tview.NewButton(" 激活 ")
	updateButton := tview.NewButton(" 更新 ")
	renameButton := tview.NewButton(" 重命名 ")
	deleteButton := tview.NewButton(" 删除 ")
	refreshButton := tview.NewButton(" 刷新 ")
	actions := tview.NewFlex().AddItem(activateButton, 10, 0, true).AddItem(updateButton, 10, 0, false).AddItem(renameButton, 12, 0, false).AddItem(deleteButton, 10, 0, false).AddItem(refreshButton, 10, 0, false)
	message := tview.NewTextView().SetDynamicColors(true)
	root := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(inputs, 3, 0, true).AddItem(table, 0, 1, false).AddItem(message, 2, 0, false).AddItem(actions, 3, 0, false)

	var mu sync.RWMutex
	var profiles []mihomotui.ProfileSummary
	selectedID := ""
	pending := false
	render := func(items []mihomotui.ProfileSummary) {
		table.Clear()
		headers := []string{"状态", "名称", "来源", "更新时间"}
		for col, h := range headers {
			table.SetCell(0, col, tview.NewTableCell(h).SetTextColor(tcell.ColorYellow).SetSelectable(false))
		}
		for i, item := range items {
			state := ""
			if item.Active {
				state = "● 活动"
			}
			table.SetCell(i+1, 0, tview.NewTableCell(state).SetTextColor(tcell.ColorGreen))
			table.SetCell(i+1, 1, tview.NewTableCell(item.Name).SetReference(item.ID))
			table.SetCell(i+1, 2, tview.NewTableCell(item.Source))
			table.SetCell(i+1, 3, tview.NewTableCell(item.UpdatedAt))
		}
		if len(items) == 0 {
			table.SetCell(1, 1, tview.NewTableCell("暂无配置，请通过 URL 导入").SetTextColor(tcell.ColorGray))
		}
		if selectedID != "" {
			for i, item := range items {
				if item.ID == selectedID {
					table.Select(i+1, 0)
					return
				}
			}
		}
		if len(items) > 0 {
			selectedID = items[0].ID
			table.Select(1, 0)
		}
	}
	refresh := func() {
		go func() {
			items, err := client.ManagerProfiles()
			if err != nil {
				reportError(app, overlay, "读取配置失败", err)
				return
			}
			mu.Lock()
			profiles = items
			mu.Unlock()
			app.QueueUpdateDraw(func() { render(items); message.SetText("") })
		}()
	}
	selected := func() (mihomotui.ProfileSummary, bool) {
		mu.RLock()
		defer mu.RUnlock()
		for _, item := range profiles {
			if item.ID == selectedID {
				return item, true
			}
		}
		return mihomotui.ProfileSummary{}, false
	}
	table.SetSelectionChangedFunc(func(row, col int) {
		if row < 1 {
			return
		}
		cell := table.GetCell(row, 1)
		if id, ok := cell.GetReference().(string); ok {
			selectedID = id
			if item, found := selected(); found {
				nameInput.SetText(item.Name)
			}
		}
	})
	do := func(label string, action func() error) {
		mu.Lock()
		if pending {
			mu.Unlock()
			return
		}
		pending = true
		mu.Unlock()
		message.SetText("[yellow]" + label + "，等待后端确认…[-]")
		go func() {
			err := action()
			mu.Lock()
			pending = false
			mu.Unlock()
			if err != nil {
				reportError(app, overlay, label+"失败", err)
				app.QueueUpdateDraw(func() { message.SetText("[red]操作失败，未修改界面状态[-]") })
				return
			}
			app.QueueUpdateDraw(func() { message.SetText("[green]" + label + "成功[-]") })
			refresh()
		}()
	}
	importButton.SetSelectedFunc(func() {
		raw := strings.TrimSpace(urlInput.GetText())
		name := strings.TrimSpace(nameInput.GetText())
		if raw == "" {
			ShowAlertModal(app, overlay, "无法导入", "配置链接不能为空")
			return
		}
		do("导入配置", func() error {
			_, err := client.ManagerImportProfile(name, raw)
			if err == nil {
				app.QueueUpdateDraw(func() { urlInput.SetText(""); nameInput.SetText("") })
			}
			return err
		})
	})
	activateButton.SetSelectedFunc(func() {
		item, ok := selected()
		if !ok {
			return
		}
		do("激活配置", func() error { _, err := client.ManagerActivateProfile(item.ID); return err })
	})
	updateButton.SetSelectedFunc(func() {
		item, ok := selected()
		if !ok {
			return
		}
		do("更新配置", func() error { _, err := client.ManagerUpdateProfile(item.ID); return err })
	})
	renameButton.SetSelectedFunc(func() {
		item, ok := selected()
		if !ok {
			return
		}
		name := strings.TrimSpace(nameInput.GetText())
		if name == "" {
			ShowAlertModal(app, overlay, "无法重命名", "请在名称框填写新名称")
			return
		}
		do("重命名", func() error { _, err := client.ManagerRenameProfile(item.ID, name); return err })
	})
	deleteButton.SetSelectedFunc(func() {
		item, ok := selected()
		if !ok {
			return
		}
		ShowConfirmModal(app, overlay, "删除配置", fmt.Sprintf("确认删除配置 %q？此操作不会删除其他配置。", item.Name), func() {
			do("删除配置", func() error { _, err := client.ManagerDeleteProfile(item.ID); return err })
		})
	})
	refreshButton.SetSelectedFunc(refresh)
	view := &pageView{Primitive: root, focusables: []tview.Primitive{nameInput, urlInput, importButton, table, activateButton, updateButton, renameButton, deleteButton, refreshButton}, first: table, refresh: refresh}
	return view
}
