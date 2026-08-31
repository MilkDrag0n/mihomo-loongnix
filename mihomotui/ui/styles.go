package ui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"mihomotui/mihomotui"
)

// configureNeutralTheme must run before widgets are constructed because tview
// copies global theme colors into each primitive at construction time.
func configureNeutralTheme() {
	tview.Styles.PrimitiveBackgroundColor = tcell.ColorBlack
	tview.Styles.ContrastBackgroundColor = tcell.NewHexColor(0x303030)
	tview.Styles.MoreContrastBackgroundColor = tcell.NewHexColor(0x505050)
	tview.Styles.BorderColor = tcell.ColorWhite
	tview.Styles.TitleColor = tcell.ColorWhite
	tview.Styles.GraphicsColor = tcell.ColorWhite
	tview.Styles.PrimaryTextColor = tcell.ColorWhite
	tview.Styles.SecondaryTextColor = tcell.ColorYellow
	tview.Styles.TertiaryTextColor = tcell.ColorGreen
	tview.Styles.InverseTextColor = tcell.ColorBlack
	tview.Styles.ContrastSecondaryTextColor = tcell.ColorSilver
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
