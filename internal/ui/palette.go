package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/SrLeet03/agentdeck/internal/session"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type paletteKind int

const (
	palAction paletteKind = iota
	palTab
	palTheme
	palSession
)

type paletteItem struct {
	kind    paletteKind
	label   string
	hint    string   // key hint shown on the right
	keys    []string // keystrokes replayed for actions
	tab     tab
	theme   string
	session *session.Session
	score   int
	pos     []int
}

const paletteRows = 12

var paletteActions = []struct {
	label, hint string
	keys        []string
}{
	{"Open / resume selected session", "enter", []string{"enter"}},
	{"New Claude Code session…", "n c", []string{"n", "c"}},
	{"New Codex session…", "n x", []string{"n", "x"}},
	{"New Pi session…", "n p", []string{"n", "p"}},
	{"Share context of selected / marked sessions…", "c", []string{"c"}},
	{"Mark / unmark session", "m", []string{"m"}},
	{"Pin / unpin session", "p", []string{"p"}},
	{"Add to group…", "+", []string{"+"}},
	{"Rename session…", "r", []string{"r"}},
	{"Edit tags…", "t", []string{"t"}},
	{"Archive / restore session", "a", []string{"a"}},
	{"Stop running session", "x", []string{"x"}},
	{"Copy resume command", "y", []string{"y"}},
	{"Toggle group by project", "g", []string{"g"}},
	{"Show / hide Codex subagents", "h", []string{"h"}},
	{"Stats", "s", []string{"s"}},
	{"Next theme", "T", []string{"T"}},
	{"Rescan session files", "R", []string{"R"}},
	{"Help: all keys", "?", []string{"?"}},
	{"Quit", "q", []string{"q"}},
}

func (m *Model) openPalette() tea.Cmd {
	m.mode = modePalette
	m.palInput.SetValue("")
	m.palInput.Focus()
	m.palCursor = 0
	m.filterPalette()
	return nil
}

func (m *Model) filterPalette() {
	q := strings.TrimSpace(m.palInput.Value())
	var items []paletteItem
	add := func(it paletteItem, text string, bonus int) {
		if q == "" {
			items = append(items, it)
			return
		}
		score, pos, ok := fuzzyMatch(q, text)
		if !ok {
			return
		}
		it.score, it.pos = score+bonus, pos
		items = append(items, it)
	}
	for _, a := range paletteActions {
		add(paletteItem{kind: palAction, label: a.label, hint: a.hint, keys: a.keys}, a.label, 4)
	}
	names := append(append([]string{}, tabNames...), m.store.Groups...)
	for i, n := range names {
		label := "Go to " + n
		if tab(i) >= tabFirstGroup {
			label = "Go to group ▸" + n
		}
		hint := ""
		if i < 9 {
			hint = fmt.Sprint(i + 1)
		}
		add(paletteItem{kind: palTab, label: label, hint: hint, tab: tab(i)}, label, 2)
	}
	for _, t := range themes {
		label := "Theme: " + t.Name
		if t.Name == theme.Name {
			label += "  (current)"
		}
		add(paletteItem{kind: palTheme, label: label, theme: t.Name}, label, 0)
	}
	for _, s := range append(append([]*session.Session{}, m.orphans...), m.sessions...) {
		if s.Subagent {
			continue
		}
		title := m.displayTitle(s)
		proj := oneLine(s.ProjectBase())
		if r := []rune(proj); len(r) > 16 {
			proj = string(r[:15]) + "…"
		}
		hint := s.Agent.Short() + " · " + proj + " · " + ago(m.activity(s))
		it := paletteItem{kind: palSession, label: title, hint: hint, session: s}
		if q == "" {
			continue // sessions appear once you type
		}
		score, pos, ok := fuzzyMatch(q, title)
		if !ok {
			if strings.Contains(strings.ToLower(s.Cwd), strings.ToLower(q)) {
				score, ok = 1, true
			}
		}
		if ok {
			it.score, it.pos = score, pos
			items = append(items, it)
		}
	}
	if q != "" {
		sort.SliceStable(items, func(i, j int) bool { return items[i].score > items[j].score })
	}
	m.palItems = items
	m.palCursor = min(m.palCursor, max(0, len(items)-1))
}

func (m Model) handlePaletteKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "ctrl+k", "ctrl+p":
		m.mode = modeNormal
		m.palInput.Blur()
		return m, nil
	case "up", "shift+tab":
		m.palCursor = max(0, m.palCursor-1)
		return m, nil
	case "down", "tab":
		m.palCursor = min(len(m.palItems)-1, m.palCursor+1)
		return m, nil
	case "enter":
		if m.palCursor < 0 || m.palCursor >= len(m.palItems) {
			return m, nil
		}
		it := m.palItems[m.palCursor]
		m.mode = modeNormal
		m.palInput.Blur()
		return m.runPaletteItem(it)
	}
	var cmd tea.Cmd
	m.palInput, cmd = m.palInput.Update(k)
	m.palCursor = 0
	m.filterPalette()
	return m, cmd
}

func (m Model) runPaletteItem(it paletteItem) (tea.Model, tea.Cmd) {
	switch it.kind {
	case palTab:
		m.setTab(it.tab)
		return m, m.ensurePreview(false)
	case palTheme:
		m.setTheme(it.theme)
		return m, nil
	case palSession:
		m.jumpTo(it.session)
		return m, m.ensurePreview(false)
	}
	var cmds []tea.Cmd
	var model tea.Model = m
	for _, k := range it.keys {
		var cmd tea.Cmd
		model, cmd = model.(Model).handleKey(keyMsg(k))
		cmds = append(cmds, cmd)
	}
	return model, tea.Batch(cmds...)
}

func keyMsg(k string) tea.KeyMsg {
	if k == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// jumpTo shows s in the list (switching tab and clearing the search if needed) and selects it.
func (m *Model) jumpTo(s *session.Session) {
	m.query = ""
	m.search.SetValue("")
	t := tabAll
	if m.store.Get(s.Key()).Archived {
		t = tabArchived
	}
	if s.Subagent {
		m.showSub = true
	}
	m.setTab(t)
	for i, r := range m.rows {
		if r.s != nil && r.s.Key() == s.Key() {
			m.cursor = i
			break
		}
	}
	m.clampScroll()
}

func (m Model) viewPalette() string {
	w := min(76, m.w-6)
	iw := w - 6 // modal border + padding
	var lines []string
	lines = append(lines, gradient("◆ command palette", colAccent, colAccent2, true, 0))
	lines = append(lines, "")
	lines = append(lines, m.palInput.View())
	lines = append(lines, stFaint.Render(strings.Repeat("─", iw)))
	if len(m.palItems) == 0 {
		lines = append(lines, stDim.Render("no matches"))
	}
	start := 0
	if m.palCursor >= paletteRows {
		start = m.palCursor - paletteRows + 1
	}
	end := min(len(m.palItems), start+paletteRows)
	for i := start; i < end; i++ {
		it := m.palItems[i]
		icon := map[paletteKind]string{palAction: "›", palTab: "⇥", palTheme: "◐", palSession: "●"}[it.kind]
		iconStyle := stAccent
		if it.kind == palSession {
			iconStyle = lipgloss.NewStyle().Foreground(agentColor(it.session.Agent))
		}
		hint := it.hint
		hw := lipgloss.Width(hint)
		lw := max(4, iw-hw-6) // icon, spaces and a 2-cell gap before the hint
		label := trunc(it.label, lw)
		if i == m.palCursor {
			line := " " + icon + " " + pad(label, lw) + " " + hint + " "
			lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(colOnAccent).Background(colAccent).Render(pad(line, iw)))
			continue
		}
		labelR := highlight(label, it.pos, stText, stAccent2.Bold(true).Underline(true))
		hintR := stDim.Render(hint)
		if it.kind == palAction {
			hintR = stKey.Render(hint)
		}
		lines = append(lines, pad(" "+iconStyle.Render(icon)+" "+labelR, iw-hw-1)+hintR+" ")
	}
	if len(m.palItems) > end {
		lines = append(lines, stDim.Render(fmt.Sprintf("  … %d more", len(m.palItems)-end)))
	}
	lines = append(lines, "", stDim.Render("↑↓ choose · enter run · esc close"))
	return stModal.Width(w - 2).Render(strings.Join(lines, "\n"))
}
