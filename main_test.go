package main

import (
	"strings"
	"testing"
)

func TestMeterUsesFractionalGlyphs(t *testing.T) {
	if got := meter(1, 16, 4, green); !strings.Contains(got, "▎") {
		t.Fatalf("expected fine-grained meter glyph, got %q", got)
	}
}

func TestDashboardViewAndPalette(t *testing.T) {
	m := model{w: 120, h: 40, mem: map[string]uint64{"MemTotal": 16 << 30, "MemAvailable": 5 << 30, "AnonPages": 6 << 30, "Cached": 2 << 30, "SReclaimable": 1 << 30}, expanded: map[string]bool{}, help: true, palette: true}
	m.groups = []group{{owner: "me", name: "firefox", pss: 2 << 30, rss: 3 << 30, ps: []proc{{pid: 22, name: "firefox", cmd: "firefox --profile test", pss: 2 << 30}}}}
	m.rows = []row{{g: 0}}
	view := m.View()
	foundRow := false
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "firefox") {
			foundRow = true
			if at := strings.Index(line, "▸ me"); at < 0 || at > 2 {
				t.Fatalf("selected row is not left-aligned: %q", line)
			}
		}
	}
	if !foundRow {
		t.Fatal("process group row missing")
	}
	if !strings.Contains(view, "COMMAND PALETTE") || !strings.Contains(view, "WTFMEM") {
		t.Fatalf("dashboard sections missing from view")
	}
}
