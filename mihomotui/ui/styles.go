package ui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"mihomotui/mihomotui"
)

// 背景和正文沿用终端默认色；只为边框、焦点和状态设置重点色。
var (
	colorBackground = tcell.ColorDefault
	colorSurface    = tcell.ColorDefault
	colorBorder     = tcell.NewHexColor(0x435051)
	colorText       = tcell.ColorDefault
	colorMuted      = tcell.NewHexColor(0x9daaa6)
	colorAccent     = tcell.NewHexColor(0x8cd5bd)
	colorSuccess    = tcell.NewHexColor(0xa4d68a)
)

// 必须在创建控件前设置；tview 会在构造时复制全局主题。
func configureNeutralTheme() {
	tview.Styles.PrimitiveBackgroundColor = colorBackground
	tview.Styles.ContrastBackgroundColor = colorSurface
	tview.Styles.MoreContrastBackgroundColor = colorBackground
	tview.Styles.BorderColor = colorBorder
	tview.Styles.TitleColor = colorMuted
	tview.Styles.GraphicsColor = colorBorder
	tview.Styles.PrimaryTextColor = colorText
	tview.Styles.SecondaryTextColor = colorAccent
	tview.Styles.TertiaryTextColor = colorSuccess
	tview.Styles.InverseTextColor = colorBackground
	tview.Styles.ContrastSecondaryTextColor = colorMuted
}

func newActionButton(label string) *tview.Button {
	button := tview.NewButton(label).
		SetStyle(tcell.StyleDefault.Foreground(colorText).Background(colorSurface)).
		SetActivatedStyle(tcell.StyleDefault.Foreground(colorAccent).Background(colorBackground).Bold(true).Underline(true))
	button.SetBorder(true)
	focusBorder(button.Box)
	return button
}

func newInputField() *tview.InputField {
	field := tview.NewInputField()
	field.SetBorder(true)
	focusBorder(field.Box)
	return field
}

func newDropDown() *tview.DropDown {
	drop := tview.NewDropDown().SetListStyles(tcell.StyleDefault, tcell.StyleDefault.Reverse(true).Bold(true))
	drop.SetBorder(true)
	focusBorder(drop.Box)
	return drop
}

func focusBorder(box *tview.Box) {
	box.SetTitleAlign(tview.AlignLeft)
	box.SetFocusFunc(func() { box.SetBorderColor(colorAccent).SetTitleColor(colorAccent) })
	box.SetBlurFunc(func() { box.SetBorderColor(colorBorder).SetTitleColor(colorMuted) })
}

func styleTable(table *tview.Table) {
	table.SetSelectedStyle(tcell.StyleDefault.Reverse(true).Bold(true))
	table.SetBorderPadding(0, 0, 1, 1)
	focusBorder(table.Box)
}

func newStatusPanel(title string) *tview.TextView {
	panel := tview.NewTextView().SetDynamicColors(true).SetWrap(true)
	panel.SetBorder(true).SetTitle(" "+title+" ").SetTitleAlign(tview.AlignLeft).SetBorderPadding(0, 0, 1, 1)
	return panel
}

// terminalCursorReset is deliberately a double-width blank. tview measures a
// regional-indicator flag cluster as two cells, while tcell 2.8 records its
// primary rune as one cell. After writing the cluster, tcell's cursor tracking is
// therefore one cell behind the target macOS terminal. Emitting a double-width rune
// makes tcell invalidate its cursor position, so the next cell is addressed with
// an absolute cursor move. Keep this in the independent region column; never mix
// flag clusters into the node-name cell again.
const terminalCursorReset = "\u3000"

func terminalNodeColumns(name string) (region, label string) {
	runes := []rune(name)
	if len(runes) < 2 || !isRegionalIndicator(runes[0]) || !isRegionalIndicator(runes[1]) {
		return terminalCursorReset, name
	}
	first, second := runes[0], runes[1]
	if first == 0x1F1F9 && second == 0x1F1FC {
		region = "TW"
	} else {
		region = string(runes[:2])
	}
	label = strings.TrimLeftFunc(string(runes[2:]), unicode.IsSpace)
	return region + terminalCursorReset, label
}

func isRegionalIndicator(value rune) bool { return value >= 0x1F1E6 && value <= 0x1F1FF }

func terminalNodeLabel(name string) string {
	runes := []rune(name)
	if len(runes) >= 2 && isRegionalIndicator(runes[0]) && isRegionalIndicator(runes[1]) {
		code := string([]rune{
			'A' + runes[0] - 0x1F1E6,
			'A' + runes[1] - 0x1F1E6,
		})
		name = code + " " + strings.TrimLeftFunc(string(runes[2:]), unicode.IsSpace)
	}
	return tview.Escape(name)
}

// DelayColor 根据延迟值返回对应的颜色标签
func DelayColor(delay int) string {
	switch {
	case delay == mihomotui.DelayUntested:
		return mihomotui.ColorMuted
	case delay == mihomotui.DelayTesting:
		return mihomotui.ColorWarn
	case delay == mihomotui.DelayTimeout:
		return mihomotui.ColorError
	case delay < 100:
		return mihomotui.ColorOK
	case delay < 200:
		return mihomotui.ColorWarn
	default:
		return mihomotui.ColorError
	}
}

// DelayText 根据延迟值返回对应的显示文本
func DelayText(delay int) string {
	switch delay {
	case mihomotui.DelayUntested:
		return "未测试"
	case mihomotui.DelayTesting:
		return "测试中"
	case mihomotui.DelayTimeout:
		return "超时"
	default:
		if delay >= 0 {
			return fmt.Sprintf("%dms", delay)
		}
		return "未知"
	}
}

// ProgressBar 生成进度条字符串
func ProgressBar(width int, percent float64) string {
	filled := 0
	if percent > 0 {
		filled = min(int(percent/100*float64(width)), width)
	}
	return fmt.Sprintf("[%s]%s[-]%s", mihomotui.ColorInfo,
		strings.Repeat("━", filled),
		strings.Repeat("─", width-filled))
}
