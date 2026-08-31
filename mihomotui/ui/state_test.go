package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"mihomotui/mihomotui"
	"strings"
	"testing"
)

func TestPagerClampsEveryBoundary(t *testing.T) {
	p := pager{Page: 99, PageSize: 10, Total: 21}
	start, end := p.Bounds()
	if p.Page != 2 || start != 20 || end != 21 {
		t.Fatalf("last page: p=%+v bounds=%d,%d", p, start, end)
	}
	p.Next()
	if p.Page != 2 {
		t.Fatalf("next escaped: %+v", p)
	}
	p.First()
	p.Prev()
	if p.Page != 0 {
		t.Fatalf("prev escaped: %+v", p)
	}
	p.Total = 0
	start, end = p.Bounds()
	if start != 0 || end != 0 || p.Pages() != 1 {
		t.Fatalf("empty: %+v %d,%d", p, start, end)
	}
}

func TestNodeAndRuleFiltersPreserveBackendSelectionData(t *testing.T) {
	nodes := []mihomotui.ProxyNode{{Name: "Hong Kong 1", Type: "ss"}, {Name: "Tokyo", Type: "vmess"}}
	filtered := filterNodes(nodes, "hong")
	if len(filtered) != 1 || filtered[0].Name != "Hong Kong 1" {
		t.Fatalf("nodes=%+v", filtered)
	}
	rules := []mihomotui.Rule{{Content: "DOMAIN,example.com", Type: "Domain", Policy: "Proxy"}, {Content: "MATCH", Type: "Match", Policy: "DIRECT"}}
	got := filterRules(rules, "direct")
	if len(got) != 1 || got[0].Content != "MATCH" {
		t.Fatalf("rules=%+v", got)
	}
}

func TestFormatBytes(t *testing.T) {
	if got := formatBytes(10 << 20); got != "10.00 MiB" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatHomeStatusIncludesRuntimePortAndCurrentNode(t *testing.T) {
	status := mihomotui.ManagerStatus{
		Core:         mihomotui.CoreRuntimeStatus{Running: true, ServiceActive: true, ControllerHealthy: true, PID: 42},
		ProxyPort:    7890,
		CurrentGroup: "Auto",
		CurrentNode:  "🇭🇰 香港 01丨1x HK",
	}
	got := tview.Unescape(formatHomeStatus(status))
	for _, want := range []string{"127.0.0.1:7890", "HTTP/SOCKS", "HK 香港 01丨1x HK", "Auto"} {
		if !strings.Contains(got, want) {
			t.Fatalf("home status missing %q: %q", want, got)
		}
	}
}

func TestTerminalNodeColumnsSeparateFlagFromPlainName(t *testing.T) {
	tests := map[string][2]string{
		"🇹🇼台湾家宽 03 | 3x TW": {"TW" + terminalCursorReset, "台湾家宽 03 | 3x TW"},
		"🇭🇰 香港 01 | 1x HK":  {"🇭🇰" + terminalCursorReset, "香港 01 | 1x HK"},
		"🇯🇵Tokyo":           {"🇯🇵" + terminalCursorReset, "Tokyo"},
		"🚀 其他节点":            {terminalCursorReset, "🚀 其他节点"},
	}
	for original, want := range tests {
		region, label := terminalNodeColumns(original)
		if region != want[0] || label != want[1] {
			t.Errorf("terminalNodeColumns(%q) = (%q, %q), want (%q, %q)", original, region, label, want[0], want[1])
		}
	}
}

func TestTerminalNodeLabelEscapesTviewTags(t *testing.T) {
	original := "[Premium] 香港"
	label := terminalNodeLabel(original)
	if label == original {
		t.Fatalf("label %q was not escaped", label)
	}
	if got := tview.Unescape(label); got != original {
		t.Fatalf("unescaped label = %q, want %q", got, original)
	}
}

func TestTerminalNodeLabelUsesWidthStableRegionCode(t *testing.T) {
	if got := tview.Unescape(terminalNodeLabel("🇭🇰 香港 01")); got != "HK 香港 01" {
		t.Fatalf("label = %q", got)
	}
}

func TestNeutralThemeContainsNoBlueBackground(t *testing.T) {
	configureNeutralTheme()
	for name, color := range map[string]tcell.Color{
		"contrast":      tview.Styles.ContrastBackgroundColor,
		"more-contrast": tview.Styles.MoreContrastBackgroundColor,
		"inverse":       tview.Styles.InverseTextColor,
	} {
		if color == tcell.ColorBlue || color == tcell.ColorNavy {
			t.Fatalf("%s color remains blue: %s", name, color)
		}
	}
}
