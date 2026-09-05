package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type pageView struct {
	tview.Primitive
	focusables []tview.Primitive
	first      tview.Primitive
	filter     tview.Primitive
	start      func()
	stop       func()
	refresh    func()
	pageKey    func(*tcell.EventKey) bool
}

func (p *pageView) Start() {
	if p.start != nil {
		p.start()
	}
	p.Refresh()
}
func (p *pageView) Stop() {
	if p.stop != nil {
		p.stop()
	}
}
func (p *pageView) Refresh() {
	if p.refresh != nil {
		p.refresh()
	}
}

func (p *pageView) focusNext(app *tview.Application, current tview.Primitive, reverse bool) {
	if len(p.focusables) == 0 {
		return
	}
	index := -1
	for i, primitive := range p.focusables {
		if primitive == current || primitive.HasFocus() {
			index = i
			break
		}
	}
	if reverse {
		index--
		if index < 0 {
			index = len(p.focusables) - 1
		}
	} else {
		index++
		if index >= len(p.focusables) {
			index = 0
		}
	}
	app.SetFocus(p.focusables[index])
}

func isTextInput(primitive tview.Primitive) bool {
	switch primitive.(type) {
	case *tview.InputField, *tview.TextArea:
		return true
	default:
		return false
	}
}

type pager struct{ Page, PageSize, Total int }

func (p *pager) normalize() {
	if p.PageSize < 1 {
		p.PageSize = 1
	}
	pages := p.Pages()
	if p.Page < 0 {
		p.Page = 0
	}
	if p.Page >= pages {
		p.Page = pages - 1
	}
}
func (p pager) Pages() int {
	if p.PageSize < 1 || p.Total == 0 {
		return 1
	}
	return (p.Total + p.PageSize - 1) / p.PageSize
}
func (p *pager) Bounds() (int, int) {
	p.normalize()
	start := p.Page * p.PageSize
	end := min(start+p.PageSize, p.Total)
	return start, end
}
func (p *pager) Next()  { p.Page++; p.normalize() }
func (p *pager) Prev()  { p.Page--; p.normalize() }
func (p *pager) First() { p.Page = 0 }
func (p *pager) Last()  { p.Page = p.Pages() - 1; p.normalize() }
