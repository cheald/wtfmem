package main

import (
	"bufio"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type proc struct {
	pid                     int
	name, cmd, owner, group string
	pss, rss, priv, virt    uint64
}
type group struct {
	owner, name          string
	ps                   []proc
	pss, rss, priv, virt uint64
}
type snap struct {
	mem    map[string]uint64
	groups []group
	err    string
}
type snapMsg snap
type tick time.Time
type row struct {
	g int
	p *proc
}
type model struct {
	w, h, cursor, scroll int
	mem                  map[string]uint64
	groups               []group
	rows                 []row
	expanded             map[string]bool
	filter               string
	searching, help      bool
	palette              bool
	paletteCursor        int
	command              string
	err                  string
}

var cyan = lipgloss.NewStyle().Foreground(lipgloss.Color("#67d5ff"))
var green = lipgloss.NewStyle().Foreground(lipgloss.Color("#5ee6a8"))
var yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("#f6c85f"))
var red = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6b6b"))
var muted = lipgloss.NewStyle().Foreground(lipgloss.Color("#8492a6"))
var picked = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#26394a")).Bold(true)

func meminfo() map[string]uint64 {
	m := map[string]uint64{}
	f, e := os.Open("/proc/meminfo")
	if e != nil {
		return m
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		p := strings.SplitN(s.Text(), ":", 2)
		if len(p) < 2 {
			continue
		}
		v := strings.Fields(p[1])
		if len(v) > 0 {
			n, _ := strconv.ParseUint(v[0], 10, 64)
			m[p[0]] = n * 1024
		}
	}
	return m
}
func smaps(path, key string) uint64 {
	f, e := os.Open(path)
	if e != nil {
		return 0
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		p := strings.SplitN(s.Text(), ":", 2)
		if len(p) == 2 && p[0] == key {
			v := strings.Fields(p[1])
			if len(v) > 0 {
				n, _ := strconv.ParseUint(v[0], 10, 64)
				return n * 1024
			}
		}
	}
	return 0
}
func scan() snap {
	m := meminfo()
	entries, e := os.ReadDir("/proc")
	if e != nil {
		return snap{mem: m, err: e.Error()}
	}
	gm := map[string]*group{}
	for _, ent := range entries {
		pid, e := strconv.Atoi(ent.Name())
		if e != nil {
			continue
		}
		base := filepath.Join("/proc", ent.Name())
		cb, e := os.ReadFile(base + "/comm")
		if e != nil {
			continue
		}
		name := strings.TrimSpace(string(cb))
		status, _ := os.ReadFile(base + "/status")
		uid := 0
		for _, l := range strings.Split(string(status), "\n") {
			if strings.HasPrefix(l, "Uid:") {
				f := strings.Fields(l)
				if len(f) > 1 {
					uid, _ = strconv.Atoi(f[1])
				}
				break
			}
		}
		owner := "system (root)"
		if uid == os.Getuid() {
			owner = "me"
		} else if uid != 0 {
			owner = "other UID " + strconv.Itoa(uid)
		}
		sm := base + "/smaps_rollup"
		rss, pss := smaps(sm, "Rss"), smaps(sm, "Pss")
		if rss == 0 {
			continue
		}
		priv := smaps(sm, "Private_Clean") + smaps(sm, "Private_Dirty")
		stat, _ := os.ReadFile(base + "/statm")
		f := strings.Fields(string(stat))
		var virt uint64
		if len(f) > 0 {
			n, _ := strconv.ParseUint(f[0], 10, 64)
			virt = n * uint64(os.Getpagesize())
		}
		cmd, _ := os.ReadFile(base + "/cmdline")
		command := strings.TrimSpace(strings.ReplaceAll(string(cmd), "\x00", " "))
		if command == "" {
			command = "[" + name + "]"
		}
		exe := name
		if p, e := os.Readlink(base + "/exe"); e == nil {
			exe = filepath.Base(p)
		}
		gname := exe
		if b, e := os.ReadFile(base + "/cgroup"); e == nil {
			for _, l := range strings.Split(string(b), "\n") {
				p := strings.SplitN(l, ":", 3)
				if len(p) < 3 {
					continue
				}
				for _, part := range strings.Split(p[2], "/") {
					if strings.HasSuffix(part, ".service") || strings.HasSuffix(part, ".scope") || strings.HasSuffix(part, ".slice") {
						gname = part
					}
				}
			}
		}
		key := owner + "\x00" + gname
		g := gm[key]
		if g == nil {
			g = &group{owner: owner, name: gname}
			gm[key] = g
		}
		p := proc{pid: pid, name: name, cmd: command, owner: owner, group: gname, pss: pss, rss: rss, priv: priv, virt: virt}
		g.ps = append(g.ps, p)
		g.pss += pss
		g.rss += rss
		g.priv += priv
		g.virt += virt
	}
	gs := make([]group, 0, len(gm))
	for _, g := range gm {
		sort.Slice(g.ps, func(i, j int) bool { return g.ps[i].pss > g.ps[j].pss })
		gs = append(gs, *g)
	}
	sort.Slice(gs, func(i, j int) bool { return gs[i].pss > gs[j].pss })
	return snap{mem: m, groups: gs}
}
func scanCmd() tea.Cmd        { return func() tea.Msg { return snapMsg(scan()) } }
func tickCmd() tea.Cmd        { return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tick(t) }) }
func (m model) Init() tea.Cmd { return tea.Batch(scanCmd(), tickCmd()) }
func (m *model) rebuild() {
	m.rows = nil
	q := strings.ToLower(m.filter)
	for i, g := range m.groups {
		match := q == "" || strings.Contains(strings.ToLower(g.owner+" "+g.name), q)
		if !match {
			for _, p := range g.ps {
				if strings.Contains(strings.ToLower(p.name+" "+p.cmd), q) {
					match = true
					break
				}
			}
		}
		if !match {
			continue
		}
		m.rows = append(m.rows, row{g: i})
		if m.expanded[g.owner+"\x00"+g.name] {
			for j := range g.ps {
				p := g.ps[j]
				if q != "" && !strings.Contains(strings.ToLower(p.name+" "+p.cmd), q) && !strings.Contains(strings.ToLower(g.owner+" "+g.name), q) {
					continue
				}
				m.rows = append(m.rows, row{g: i, p: &p})
			}
		}
	}
	if len(m.rows) == 0 {
		m.cursor = 0
	} else if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	m.setCommand()
}
func (m *model) setCommand() {
	m.command = "Select an expanded process row to inspect its full command line."
	if m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].p != nil {
		p := m.rows[m.cursor].p
		m.command = fmt.Sprintf("%s (PID %d): %s", p.name, p.pid, p.cmd)
	}
}
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = x.Width, x.Height
	case snapMsg:
		m.mem = x.mem
		m.groups = x.groups
		m.err = x.err
		m.rebuild()
	case tick:
		return m, tea.Batch(scanCmd(), tickCmd())
	case tea.KeyMsg:
		k := x.String()
		if m.palette {
			switch k {
			case "esc", "ctrl+p":
				m.palette = false
			case "up", "k":
				m.paletteCursor = (m.paletteCursor + 3) % 4
			case "down", "j":
				m.paletteCursor = (m.paletteCursor + 1) % 4
			case "enter":
				m.palette = false
				switch m.paletteCursor {
				case 0:
					m.searching = true
				case 1:
					return m, scanCmd()
				case 2:
					m.help = !m.help
				case 3:
					return m, tea.Quit
				}
			}
			return m, nil
		}
		if m.searching {
			switch k {
			case "esc":
				m.searching = false
				m.filter = ""
				m.cursor = 0
				m.rebuild()
			case "enter":
				m.searching = false
			case "backspace", "ctrl+h":
				if len(m.filter) > 0 {
					m.filter = m.filter[:len(m.filter)-1]
					m.cursor = 0
					m.rebuild()
				}
			default:
				if len(k) == 1 {
					m.filter += k
					m.cursor = 0
					m.rebuild()
				}
			}
			return m, nil
		}
		switch k {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			return m, scanCmd()
		case "ctrl+p":
			m.palette = true
		case "/":
			m.searching = true
		case "?":
			m.help = !m.help
		case "esc":
			if m.filter != "" {
				m.filter = ""
				m.cursor = 0
				m.rebuild()
			}
			m.help = false
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			m.setCommand()
		case "down", "j":
			if m.cursor < len(m.rows)-1 {
				m.cursor++
			}
			m.setCommand()
		case "pgup":
			m.cursor = max(0, m.cursor-m.visible())
			m.setCommand()
		case "pgdown":
			m.cursor = min(len(m.rows)-1, m.cursor+m.visible())
			m.setCommand()
		case "enter":
			if m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].p == nil {
				g := m.groups[m.rows[m.cursor].g]
				key := g.owner + "\x00" + g.name
				m.expanded[key] = !m.expanded[key]
				m.rebuild()
			}
		}
	}
	return m, nil
}
func (m model) visible() int { return max(1, m.h-16) }
func human(n uint64) string {
	v := float64(n)
	for _, u := range []string{"B", "KiB", "MiB", "GiB", "TiB"} {
		if v < 1024 || u == "TiB" {
			return fmt.Sprintf("%.1f %s", v, u)
		}
		v /= 1024
	}
	return ""
}
func clip(s string, w int) string {
	if w < 1 {
		return ""
	}
	r := []rune(s)
	if len(r) > w {
		if w > 1 {
			return string(r[:w-1]) + "…"
		}
		return string(r[:w])
	}
	return s + strings.Repeat(" ", w-len(r))
}
func largestProcessPSS(g group) uint64 {
	if len(g.ps) == 0 {
		return max(uint64(1), g.pss)
	}
	largest := uint64(1)
	for _, p := range g.ps {
		largest = max(largest, p.pss)
	}
	return largest
}

func meter(v, max uint64, w int, color lipgloss.Style) string {
	if max == 0 {
		max = 1
	}
	if w <= 0 {
		return ""
	}
	levels := []string{"▏", "▎", "▍", "▌", "▋", "▊", "▉", "█"}
	amount := min(float64(w), float64(v)/float64(max)*float64(w))
	whole := int(amount)
	fraction := amount - float64(whole)
	var out strings.Builder
	if whole > 0 {
		out.WriteString(color.Render(strings.Repeat("█", whole)))
	}
	partial := 0
	if fraction > 0 && whole < w {
		partial = int(fraction*8 + 0.999)
		partial = min(8, partial)
		out.WriteString(color.Render(levels[partial-1]))
	}
	remaining := w - whole
	if partial > 0 {
		remaining--
	}
	if remaining > 0 {
		out.WriteString(muted.Render(strings.Repeat("·", remaining)))
	}
	return out.String()
}
func wrap(s string, w int) []string {
	if w < 1 {
		return []string{}
	}
	words := strings.Fields(s)
	var out []string
	line := ""
	for _, word := range words {
		for len([]rune(word)) > w {
			r := []rune(word)
			if line != "" {
				out = append(out, line)
				line = ""
			}
			out = append(out, string(r[:w]))
			word = string(r[w:])
		}
		if line == "" {
			line = word
		} else if len([]rune(line))+1+len([]rune(word)) <= w {
			line += " " + word
		} else {
			out = append(out, line)
			line = word
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}
func compositionBar(mem map[string]uint64, width int) string {
	total := max(uint64(1), mem["MemTotal"])
	fileCache := uint64(0)
	if mem["Cached"] > mem["Shmem"] {
		fileCache = mem["Cached"] - mem["Shmem"]
	}
	parts := []struct {
		value uint64
		style lipgloss.Style
	}{
		{mem["AnonPages"], red},
		{fileCache, cyan},
		{mem["Shmem"], lipgloss.NewStyle().Foreground(lipgloss.Color("#c792ea"))},
		{mem["SReclaimable"] + mem["SUnreclaim"], yellow},
	}
	var out strings.Builder
	used := 0
	for _, p := range parts {
		n := int(float64(width) * float64(p.value) / float64(total))
		n = min(n, width-used)
		if n > 0 {
			out.WriteString(p.style.Render(strings.Repeat("█", n)))
			used += n
		}
	}
	if used < width {
		out.WriteString(muted.Render(strings.Repeat("░", width-used)))
	}
	return out.String()
}

func (m model) View() string {
	if m.w == 0 {
		return "Starting wtfmem…"
	}
	var b strings.Builder
	total, avail := m.mem["MemTotal"], m.mem["MemAvailable"]
	used := uint64(0)
	if total > avail {
		used = total - avail
	}
	b.WriteString(cyan.Bold(true).Render("WTFMEM   MEMORY DASHBOARD") + "\n")
	summary := fmt.Sprintf("RAM  %s total    %s available    %s in use (%4.1f%%)    Swap %s / %s", human(total), green.Render(human(avail)), red.Render(human(used)), float64(used)*100/float64(max(uint64(1), total)), human(m.mem["SwapTotal"]-m.mem["SwapFree"]), human(m.mem["SwapTotal"]))
	b.WriteString(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#38566e")).Padding(0, 1).Render(summary) + "\n")
	bw := max(1, m.w-2)
	b.WriteString(compositionBar(m.mem, bw) + "\n")
	sh := m.mem["Shmem"]
	cache := m.mem["Cached"]
	if cache > sh {
		cache -= sh
	} else {
		cache = 0
	}
	b.WriteString(fmt.Sprintf("%s %s   %s ~%s   %s %s   %s %s\n", red.Render("● Anon"), human(m.mem["AnonPages"]), cyan.Render("● Cache"), human(cache), lipgloss.NewStyle().Foreground(lipgloss.Color("#c792ea")).Render("● Shared"), human(sh), yellow.Render("● Slab"), human(m.mem["SReclaimable"]+m.mem["SUnreclaim"])))
	b.WriteString(muted.Render(fmt.Sprintf("Buffers %s  reclaimable slab %s  unreclaimable slab %s  (Available is best pressure signal; cache is often reclaimable.)", human(m.mem["Buffers"]), human(m.mem["SReclaimable"]), human(m.mem["SUnreclaim"]))) + "\n")
	if m.searching {
		b.WriteString(yellow.Render("Search: "+m.filter+"▏") + "\n")
	} else if m.filter != "" {
		b.WriteString(yellow.Render("Filter: "+m.filter) + "\n")
	} else {
		b.WriteString(muted.Render("/ filter   Ctrl+P command palette   Enter expand/collapse   ? help   r refresh   q quit") + "\n")
	}
	if m.help {
		help := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#f6c85f")).Padding(0, 1).Width(max(20, m.w-4)).Render("KEYBOARD SHORTCUTS\n↑/↓ or j/k  Navigate    Enter  Expand/collapse    /  Filter    Ctrl+P  Command palette\nEsc  Clear/close    r  Refresh    q  Quit")
		b.WriteString(help + "\n")
	}
	if m.palette {
		items := []string{"Search processes", "Refresh memory snapshot", "Toggle keyboard help", "Quit wtfmem"}
		var menu strings.Builder
		menu.WriteString(cyan.Bold(true).Render("COMMAND PALETTE") + "  " + muted.Render("↑↓ select · Enter run · Esc close") + "\n")
		for i, item := range items {
			line := "  " + item
			if i == m.paletteCursor {
				line = "› " + item
				menu.WriteString(picked.Render(line) + "\n")
			} else {
				menu.WriteString(muted.Render(line) + "\n")
			}
		}
		b.WriteString(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#67d5ff")).Padding(0, 1).Width(max(20, m.w-4)).Render(menu.String()) + "\n")
	}
	if m.err != "" {
		b.WriteString(red.Render("Scan error: "+m.err) + "\n")
	}
	compact := m.w < 112
	header := ""
	if compact {
		header = fmt.Sprintf("%-18s %-*s %11s  %s\n", "OWNER", max(8, m.w-43), "GROUP / PROCESS", "PSS", "RELATIVE")
	} else {
		groupWidth := max(24, min(72, m.w-85))
		header = fmt.Sprintf("%-16s %-*s %5s %10s %-21s %9s %9s %9s\n", "OWNER", groupWidth, "GROUP / PROCESS", "COUNT", "PSS", "SHARE", "RSS", "PRIVATE", "VIRT")
	}
	b.WriteString(muted.Bold(true).Render(strings.TrimSuffix(header, "\n")) + "\n")
	b.WriteString(muted.Render(strings.Repeat("─", max(1, m.w))) + "\n")
	fixed := 13
	if m.searching {
		fixed++
	}
	if m.help {
		fixed += 4
	}
	if m.palette {
		fixed += 7
	}
	listH := max(1, m.h-fixed-5)
	listH = min(listH, len(m.rows))
	if m.cursor < m.scroll {
		m.scroll = m.cursor
	}
	if m.cursor >= m.scroll+listH {
		m.scroll = m.cursor - listH + 1
	}
	maxP := uint64(1)
	sum := uint64(1)
	for _, g := range m.groups {
		maxP = max(maxP, g.pss)
		sum += g.pss
	}
	end := min(len(m.rows), m.scroll+listH)
	for i := m.scroll; i < end; i++ {
		r := m.rows[i]
		g := m.groups[r.g]
		var text string
		if compact {
			label := g.owner + " / " + g.name
			ps := human(g.pss)
			val, scale, shareTotal, meterColor := g.pss, maxP, sum, green
			if r.p != nil {
				label = "  " + r.p.name + fmt.Sprintf(" (PID %d)", r.p.pid)
				ps = human(r.p.pss)
				val, scale, shareTotal, meterColor = r.p.pss, largestProcessPSS(g), max(uint64(1), g.pss), cyan
			}
			lead := fmt.Sprintf("%-18s %-*s %11s  ", clip(func() string {
				if r.p != nil {
					return "process"
				}
				if m.expanded[g.owner+"\x00"+g.name] {
					return "▾ " + g.owner
				}
				return "▸ " + g.owner
			}(), 18), max(8, m.w-43), clip(label, max(8, m.w-43)), ps)
			mw := min(20, max(3, m.w-len([]rune(lead))-7))
			text = lead + meter(val, scale, mw, meterColor) + fmt.Sprintf(" %4.1f%%", float64(val)*100/float64(shareTotal))
		} else {
			groupWidth := max(24, min(72, m.w-85))
			owner, label, count := "  process", "", ""
			var pss, rss, priv, virt, scale, shareTotal uint64
			meterColor := green
			if r.p != nil {
				label = r.p.name + fmt.Sprintf(" (PID %d)", r.p.pid)
				pss, rss, priv, virt = r.p.pss, r.p.rss, r.p.priv, r.p.virt
				scale, shareTotal, meterColor = largestProcessPSS(g), max(uint64(1), g.pss), cyan
			} else {
				owner = "▸ " + g.owner
				if m.expanded[g.owner+"\x00"+g.name] {
					owner = "▾ " + g.owner
				}
				label, count = g.name, strconv.Itoa(len(g.ps))
				pss, rss, priv, virt = g.pss, g.rss, g.priv, g.virt
				scale, shareTotal = maxP, sum
			}
			share := fmt.Sprintf("%4.1f%%", float64(pss)*100/float64(shareTotal))
			text = fmt.Sprintf("%-16s %-*s %5s %10s %-13s %9s %9s %9s", clip(owner, 16), groupWidth, clip(label, groupWidth), count, human(pss), meter(pss, scale, 14, meterColor)+" "+share, human(rss), human(priv), human(virt))
		}
		if i == m.cursor {
			text = picked.Render(text)
		}
		b.WriteString(text + "\n")
	}
	cmd := wrap(m.command, max(1, m.w-8))
	room := max(1, min(3, m.h-2))
	if len(cmd) > room {
		cmd = append(cmd[:room-1], "… (command continues)")
	}
	commandPanel := cyan.Bold(true).Render("⌕  SELECTION / FULL COMMAND LINE") + "\n" + strings.Join(cmd, "\n")
	b.WriteString(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#38566e")).Padding(0, 1).Width(max(12, m.w-4)).Render(commandPanel) + "\n")
	b.WriteString(muted.Render("PSS apportions shared pages · RSS overlaps · VIRT is address space, not RAM · process and kernel metrics overlap."))
	return b.String()
}
func main() {
	p := tea.NewProgram(model{expanded: map[string]bool{}, w: 100, h: 40}, tea.WithAltScreen())
	if _, e := p.Run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
