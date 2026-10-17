package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/SrLeet03/agentdeck/internal/live"
	"github.com/SrLeet03/agentdeck/internal/session"
	"github.com/SrLeet03/agentdeck/internal/term"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) listWidth() int  { return max(44, m.w*56/100) }
func (m Model) bodyHeight() int { return max(3, m.h-3) } // header + tabs + footer
func (m Model) listHeight() int { return m.bodyHeight() - 2 }

func (m Model) View() string {
	if m.w == 0 {
		return ""
	}
	if m.w < 60 || m.h < 12 {
		return "agentdeck needs at least 60×12 — enlarge the window"
	}
	var body string
	switch m.mode {
	case modeHelp:
		body = m.viewHelp()
	case modeStats:
		body = m.viewStats()
	default:
		lw := m.listWidth()
		rw := m.w - lw
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.viewList(lw), m.viewPreview(rw))
	}
	return fitScreen(lipgloss.JoinVertical(lipgloss.Left, m.viewHeader(), m.viewTabs(), body, m.viewFooter()), m.w, m.h)
}

// fitScreen guarantees the frame never exceeds the window; an oversized frame scrolls the
// terminal and leaves the dashboard garbled.
func fitScreen(frame string, w, h int) string {
	lines := strings.Split(frame, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		if lipgloss.Width(l) > w {
			lines[i] = trunc(l, w)
		}
	}
	return strings.Join(lines, "\n")
}

// ---- header / tabs / footer -------------------------------------------------

func (m Model) viewHeader() string {
	working, idle := 0, 0
	for _, p := range m.liveBy {
		if p.Status == live.Working {
			working++
		} else {
			idle++
		}
	}
	today := time.Now().Format("2006-01-02")
	var todayTok, weekTok int64
	todaySess := 0
	weekAgo := time.Now().AddDate(0, 0, -6).Format("2006-01-02")
	for _, s := range m.sessions {
		if s.Daily[today] > 0 || m.activity(s).Format("2006-01-02") == today {
			todaySess++
		}
		for d, n := range s.Daily {
			if d == today {
				todayTok += n
			}
			if d >= weekAgo {
				weekTok += n
			}
		}
	}
	sep := stFaint.Render("  │  ")
	parts := []string{
		stGreen.Render(fmt.Sprintf("● %d working", working)) + "  " + stYellow.Render(fmt.Sprintf("◐ %d idle", idle)),
		stBold.Render(fmt.Sprint(len(m.sessions))) + stDim.Render(" sessions · ") + stBold.Render(fmt.Sprint(todaySess)) + stDim.Render(" active today"),
		stDim.Render("tokens today ") + stBold.Render(humanTokens(todayTok)) + stDim.Render(" · 7d ") + stBold.Render(humanTokens(weekTok)),
	}
	if m.loading {
		parts = append(parts, stDim.Render("scanning…"))
	}
	line := stTitle.Render("◆ agentdeck") + "  " + strings.Join(parts, sep)
	return trunc(line, m.w)
}

func (m Model) tabCount(t tab) int {
	saved := m.tab
	m.tab = t
	n := 0
	terms := strings.Fields(strings.ToLower(m.query))
	for _, s := range m.orphans {
		if m.visible(s, terms) {
			n++
		}
	}
	for _, s := range m.sessions {
		if m.visible(s, terms) {
			n++
		}
	}
	m.tab = saved
	return n
}

func (m Model) viewTabs() string {
	var parts []string
	names := append(append([]string{}, tabNames...), m.store.Groups...)
	for i, name := range names {
		num := ""
		if i < 9 {
			num = fmt.Sprintf("%d ", i+1)
		}
		isGroup := tab(i) >= tabFirstGroup
		if isGroup {
			name = "▸" + name
		}
		label := fmt.Sprintf("%s%s %d", num, name, m.tabCount(tab(i)))
		switch {
		case tab(i) == m.tab:
			parts = append(parts, stTabOn.Render(label))
		case isGroup:
			parts = append(parts, stTabOff.Foreground(colAccent).Render(label))
		default:
			parts = append(parts, stTabOff.Render(label))
		}
	}
	// Many groups: scroll the tab strip so the current tab stays visible.
	first := 0
	for first < int(m.tab) && lipgloss.Width(strings.Join(parts[first:int(m.tab)+1], "")) > m.w-24 {
		first++
	}
	left := strings.Join(parts[first:], "")
	if first > 0 {
		left = stDim.Render("‹ ") + left
	}
	var right string
	switch {
	case m.mode == modeSearch:
		right = m.search.View()
	case m.query != "":
		right = stAccent.Render("/ "+m.query) + stDim.Render("  esc clears")
	default:
		right = stDim.Render("/ search")
	}
	if m.grouped {
		right = stDim.Render("[by project] ") + right
	}
	gap := m.w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return trunc(left, m.w)
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) viewFooter() string {
	switch m.mode {
	case modeRename, modeTags, modeNewDir, modeShareDir, modeGroupAdd, modeGroupRename:
		return trunc(m.input.View()+stDim.Render("   enter ok · esc cancel"), m.w)
	case modeConfirmGroupDelete:
		g, _ := m.currentGroup()
		return stRed.Render("Delete group "+g+"? ") + stDim.Render("(sessions stay, only the group goes) ") + key("y") + stDim.Render(" yes  any other key cancels")
	case modeShareMenu:
		return trunc(stBold.Render(fmt.Sprintf("Share context of %s → ", plural(len(m.share), "session")))+
			key("n")+" new session  "+key("s")+" send to a running session  "+key("y")+" copy to clipboard   "+stDim.Render("esc cancel"), m.w)
	case modeShareAgent:
		return stBold.Render("New session with context: ") + key("c") + " Claude Code  " + key("x") + " Codex  " + key("p") + " Pi   " + stDim.Render("esc cancel")
	case modePickTarget:
		return trunc(stBold.Render("Send context to: ")+stDim.Render("move to a running session (●/◐) and press ")+key("enter")+stDim.Render(" · esc cancel"), m.w)
	case modeNewAgent:
		return stBold.Render("New session: ") + key("c") + " Claude Code  " + key("x") + " Codex  " + key("p") + " Pi   " + stDim.Render("esc cancel")
	case modeConfirmStop:
		s := m.selected()
		what := ""
		if s != nil {
			if p := m.liveBy[s.Key()]; p != nil {
				what = fmt.Sprintf("%s pid %d", p.Agent, p.PID)
				if p.Agent == "claude" && p.Kind == "bg" {
					what = "background Claude job " + p.JobID
				}
			}
		}
		return stRed.Render("Stop "+what+"? ") + key("y") + stDim.Render(" yes  any other key cancels")
	case modeHelp, modeStats:
		return stDim.Render("esc back · q quit")
	}
	if m.status != "" && time.Since(m.statusAt) < 6*time.Second {
		if m.statusErr {
			return trunc(stRed.Render("✗ "+m.status), m.w)
		}
		return trunc(stGreen.Render("✓ ")+m.status, m.w)
	}
	if m.busy != "" {
		spin := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}[m.ticks%10]
		return trunc(stAccent.Render(spin+" "+m.busy+"… ")+stDim.Render(fmt.Sprintf("%ds · keep working, it opens when ready", int(time.Since(m.busySince).Seconds()))), m.w)
	}
	if len(m.marked) > 0 {
		return trunc(stAccent.Render(fmt.Sprintf("✓ %s marked", plural(len(m.marked), "session")))+stDim.Render(" · ")+
			key("c")+" share their context"+stDim.Render(" · ")+key("m")+" toggle"+stDim.Render(" · ")+key("esc")+" clear", m.w)
	}
	if g, ok := m.currentGroup(); ok {
		return trunc(stAccent.Render("▸ "+g)+stDim.Render(" · ")+key("enter")+" open"+stDim.Render(" · ")+key("+")+" add"+stDim.Render(" · ")+
			key("-")+" remove from group"+stDim.Render(" · ")+key("E")+" rename group"+stDim.Render(" · ")+key("X")+" delete group"+stDim.Render(" · ")+
			key("c")+" share ctx"+stDim.Render(" · ")+key("?")+" help", m.w)
	}
	hints := []string{key("enter") + " open", key("n") + " new", key("p") + " pin", key("+") + " group", key("c") + " share ctx", key("m") + " mark", key("/") + " search", key("g") + " by project",
		key("r") + " rename", key("t") + " tag", key("a") + " archive", key("x") + " stop",
		key("y") + " copy cmd", key("s") + " stats", key("?") + " help", key("q") + " quit"}
	return trunc(strings.Join(hints, stFaint.Render(" · ")), m.w)
}

func key(k string) string { return stKey.Render(k) }

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// ---- list -------------------------------------------------------------------

func (m Model) viewList(w int) string {
	iw := w - 2
	h := m.listHeight()
	var lines []string
	if !m.loaded {
		lines = append(lines, stDim.Render(" loading sessions… (first run parses everything, later runs are cached)"))
	} else if len(m.rows) == 0 {
		msg := " no sessions here"
		if m.query != "" {
			msg = " nothing matches \"" + m.query + "\""
		}
		lines = append(lines, stDim.Render(msg))
	}
	end := min(len(m.rows), m.offset+h)
	for i := m.offset; i < end; i++ {
		lines = append(lines, m.renderRow(m.rows[i], iw, i == m.cursor))
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	// scroll indicator in the last line
	if len(m.rows) > h {
		pos := fmt.Sprintf(" %d/%d ", m.cursor+1, len(m.rows))
		last := lines[h-1]
		if lipgloss.Width(last) == 0 || m.cursor != end-1 {
			lines[h-1] = pad(last, iw-len(pos)) + stDim.Render(pos)
		}
	}
	return stPaneOn.Width(iw).Height(h).Render(strings.Join(lines, "\n"))
}

func (m Model) renderRow(r row, w int, sel bool) string {
	if r.section {
		arrow := "▾"
		if m.collapsed[r.header] {
			arrow = "▸"
		}
		label := fmt.Sprintf("%s %s  %d", arrow, r.header, r.count)
		if sel {
			return lipgloss.NewStyle().Bold(true).Foreground(colText).Background(colSelBg).Render(pad(label, w))
		}
		st := stDim.Bold(true)
		if r.header == secPinned {
			st = stYellow.Bold(true)
		}
		rule := strings.Repeat("─", max(0, w-lipgloss.Width(label)-1))
		return st.Render(label) + " " + stFaint.Render(rule)
	}
	if r.header != "" {
		arrow := "▾"
		if m.collapsed[r.header] {
			arrow = "▸"
		}
		right := fmt.Sprintf("%d  %4s", r.count, ago(r.latest))
		left := arrow + " " + session.ShortPath(r.header)
		txt := pad(left, w-len(right)-1) + " " + right
		if sel {
			return lipgloss.NewStyle().Bold(true).Foreground(colText).Background(colSelBg).Render(pad(txt, w))
		}
		return stSection.Render(pad(left, w-len(right)-1)) + " " + stDim.Render(right)
	}
	s := r.s
	meta := m.store.Get(s.Key())
	p := m.liveBy[s.Key()]
	indent := ""
	if m.grouped {
		indent = "  "
	}
	pin := " "
	if meta.Pinned {
		pin = "★"
	}
	marked := m.marked[s.Key()]
	if marked {
		pin = "✓"
	}
	when := fmt.Sprintf("%4s", ago(m.activity(s)))
	projW := min(26, max(10, w/4))
	if m.grouped {
		projW = min(18, max(8, w/6))
	}
	proj := oneLine(s.ProjectBase())
	if m.grouped {
		proj = oneLine(worktreeName(s.Cwd))
	}
	title := m.displayTitle(s)
	tags := ""
	for _, t := range meta.Tags {
		tags += " #" + t
	}
	fixed := len(indent) + 1 + 1 + 2 + 1 + 1 + 1 + projW + 1 + len(when)
	titleW := max(8, w-fixed-1)

	if sel {
		g := " "
		if p != nil {
			g = map[live.Status]string{live.Working: "●", live.Idle: "◐", live.Shell: "$"}[p.Status]
			if g == "" {
				g = "○"
			}
		}
		txt := indent + g + " " + s.Agent.Short() + " " + pin + " " + pad(title+tags, titleW) + " " + pad(proj, projW) + " " + when
		return lipgloss.NewStyle().Bold(true).Foreground(colText).Background(colSelBg).Render(pad(txt, w))
	}
	tl := pad(title, titleW)
	if tags != "" && lipgloss.Width(title) < titleW-2 {
		tl = title + stAccent.Render(trunc(tags, titleW-lipgloss.Width(title)))
		tl = pad(tl, titleW)
	}
	titleStyle := lipgloss.NewStyle().Foreground(colText)
	if s.Subagent {
		titleStyle = stDim
	}
	if p == nil && time.Since(m.activity(s)) > 7*24*time.Hour {
		titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	}
	pinStyle := stYellow
	if marked {
		pinStyle = stAccent.Bold(true)
	}
	return indent + statusGlyph(p) + " " + agentBadge(s.Agent) + " " + pinStyle.Render(pin) + " " +
		titleStyle.Render(tl) + " " + stDim.Render(pad(proj, projW)) + " " + stDim.Render(when)
}

func worktreeName(cwd string) string {
	if i := strings.Index(cwd, "/.claude/worktrees/"); i >= 0 {
		return "wt:" + cwd[i+len("/.claude/worktrees/"):]
	}
	if i := strings.Index(cwd, "/.worktrees/"); i >= 0 {
		return "wt:" + cwd[i+len("/.worktrees/"):]
	}
	return ""
}

// ---- preview ----------------------------------------------------------------

func (m Model) viewPreview(w int) string {
	iw := w - 4 // border + 1 col padding each side
	h := m.bodyHeight() - 2
	style := stPane.Width(w-2).Height(h).Padding(0, 1)
	r := m.cur()
	if r == nil {
		return style.Render(stDim.Render("nothing selected"))
	}
	if r.section {
		txt := stBold.Render(r.header) + "\n\n" + stDim.Render(fmt.Sprintf("%s · enter/space folds this section", plural(r.count, "session")))
		if r.header == secPinned {
			txt += "\n" + stDim.Render("p on a session pins or unpins it")
		}
		return style.Render(txt)
	}
	if r.header != "" {
		return style.Render(m.groupSummary(r.header, iw))
	}
	s := r.s
	meta := m.store.Get(s.Key())
	p := m.liveBy[s.Key()]

	var head []string
	head = append(head, lipgloss.NewStyle().Bold(true).Foreground(agentColor(s.Agent)).Render(agentName(s.Agent))+
		stDim.Render("  "+oneLine(s.Source)))
	head = append(head, clip(wrap(stBold.Render(m.displayTitle(s)), iw), 3)...)
	if meta.Name != "" && oneLine(s.DisplayTitle()) != meta.Name {
		head = append(head, stDim.Render(trunc("orig: "+oneLine(s.DisplayTitle()), iw)))
	}
	kv := func(k, v string) { head = append(head, trunc(stDim.Render(fmt.Sprintf("%-9s", k))+v, iw)) }
	if p != nil {
		info := statusLabel(p) + stDim.Render(fmt.Sprintf("  pid %d", p.PID))
		switch {
		case p.Agent == "claude" && p.Kind == "bg":
			info += stDim.Render("  bg job " + p.JobID)
			if p.AttachedTTY != "" {
				info += stDim.Render(" · attached " + p.AttachedTTY)
			}
		case p.TTY != "":
			info += stDim.Render("  " + p.TTY)
		}
		kv("Live", info)
	}
	kv("Folder", oneLine(session.ShortPath(s.Cwd)))
	if s.Branch != "" {
		kv("Branch", oneLine(s.Branch))
	}
	if s.Model != "" {
		kv("Model", oneLine(s.Model))
	}
	kv("Started", stamp(s.Created))
	rel := ago(m.activity(s))
	if rel == "now" {
		rel = "just now"
	} else if !strings.Contains(rel, " ") {
		rel += " ago"
	}
	kv("Active", stamp(m.activity(s))+stDim.Render("  ("+rel+")"))
	msgs := fmt.Sprintf("%d you · %d agent", s.UserMsgs, s.AgentMsgs)
	if s.Subagents > 0 {
		msgs += fmt.Sprintf(" · %d subagents", s.Subagents)
	}
	kv("Messages", msgs)
	t := s.Tokens
	kv("Tokens", stBold.Render(humanTokens(t.Total()))+stDim.Render(fmt.Sprintf("  in %s · out %s · cache r %s w %s",
		humanTokens(t.In), humanTokens(t.Out), humanTokens(t.CacheRead), humanTokens(t.CacheWrite))))
	if len(meta.Tags) > 0 {
		kv("Tags", stAccent.Render("#"+strings.Join(meta.Tags, " #")))
	}
	if len(meta.Groups) > 0 {
		kv("Groups", stAccent.Render("▸"+strings.Join(meta.Groups, "  ▸")))
	}
	if s.Path != "" {
		kv("Resume", stDim.Render(baseResumeCommand(s)))
	}
	if _, exact := term.ExistingDir(s.Cwd); !exact && s.Cwd != "" {
		head = append(head, stYellow.Render(trunc("⚠ folder no longer exists (deleted worktree?)", iw)))
	}

	pd := m.preview[s.Key()]
	sec := "── conversation "
	if pd != nil && pd.truncated {
		sec += "(latest part of a large log) "
	}
	if m.pScroll > 0 {
		sec += fmt.Sprintf("↑%d ", m.pScroll)
	}
	head = append(head, stFaint.Render(trunc(sec+strings.Repeat("─", max(0, iw-lipgloss.Width(sec))), iw)))
	var conv []string
	switch {
	case s.Path == "":
		conv = []string{stDim.Render("no transcript yet")}
	case pd == nil:
		conv = []string{stDim.Render("loading…")}
	default:
		conv = renderConversation(pd.msgs, s.Agent, iw)
		if len(conv) == 0 {
			conv = []string{stDim.Render("no messages")}
		}
	}
	// Short windows: the details block may not fit, so it gets trimmed and the conversation hidden.
	if len(head) > h {
		head = head[:h]
	}
	convH := max(0, h-len(head))
	// Show the tail, scrolled back by pScroll lines.
	endIdx := len(conv) - min(m.pScroll, max(0, len(conv)-convH))
	start := max(0, endIdx-convH)
	view := conv[start:endIdx]
	return style.Render(strings.Join(append(head, view...), "\n"))
}

func renderConversation(msgs []session.Message, a session.Agent, w int) []string {
	var out []string
	agentStyle := lipgloss.NewStyle().Foreground(agentColor(a)).Bold(true)
	for _, msg := range msgs {
		switch msg.Role {
		case "user":
			out = append(out, "", stAccent.Bold(true).Render("you")+stDim.Render(" "+clock(msg.At)))
			out = append(out, clip(wrap(strings.TrimSpace(msg.Text), w), 14)...)
		case "assistant":
			out = append(out, "", agentStyle.Render(a.Short())+stDim.Render(" "+clock(msg.At)))
			out = append(out, clip(wrap(strings.TrimSpace(msg.Text), w), 18)...)
		case "tool":
			out = append(out, stFaint.Render(trunc("  ⚙ "+oneLine(msg.Text), w)))
		}
	}
	return out
}

func clock(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if time.Since(t) > 20*time.Hour {
		return t.Local().Format("Jan 2 15:04")
	}
	return t.Local().Format("15:04")
}

func wrap(s string, w int) []string {
	if w <= 0 {
		return nil
	}
	return strings.Split(lipgloss.NewStyle().Width(w).Render(clean(s)), "\n")
}

func clip(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return append(lines[:n:n], stDim.Render(fmt.Sprintf("… %d more lines", len(lines)-n)))
}

func (m Model) groupSummary(project string, w int) string {
	var n, live int
	var tok int64
	agents := map[session.Agent]int{}
	var latest time.Time
	for _, s := range m.sessions {
		if projectKey(s.Cwd) != project {
			continue
		}
		n++
		agents[s.Agent]++
		tok += s.Tokens.Total()
		if m.liveBy[s.Key()] != nil {
			live++
		}
		if a := m.activity(s); a.After(latest) {
			latest = a
		}
	}
	lines := []string{stBold.Render(session.ShortPath(project)), ""}
	lines = append(lines, stDim.Render("Sessions  ")+fmt.Sprint(n)+stDim.Render(fmt.Sprintf("   (%s %d · %s %d · %s %d)",
		"CC", agents[session.Claude], "CX", agents[session.Codex], "PI", agents[session.Pi])))
	lines = append(lines, stDim.Render("Live      ")+fmt.Sprint(live))
	lines = append(lines, stDim.Render("Tokens    ")+humanTokens(tok))
	lines = append(lines, stDim.Render("Active    ")+stamp(latest))
	lines = append(lines, "", stDim.Render("enter/space fold · n new session here"))
	for i := range lines {
		lines[i] = trunc(lines[i], w)
	}
	return strings.Join(lines, "\n")
}

// ---- help -------------------------------------------------------------------

func (m Model) viewHelp() string {
	rows := [][2]string{
		{"↑/↓  j/k", "move  ·  pgup/pgdn ctrl+u/d  ·  home/end"},
		{"tab / 1-9", "switch tab: All, Pinned, Live, Claude, Codex, Pi, Archived, then your groups (▸)"},
		{"enter", "running → jump to its tab (or attach a Claude bg job); otherwise resume in a new tab (" + term.Name() + ")"},
		{"n", "new session: pick agent, then folder (defaults to the selected session's folder)"},
		{"/", "search title, prompts, folder, model, branch;  #tag filters by tag;  @group by group;  esc clears"},
		{"g", "toggle grouping by project (worktrees fold into their repo)  ·  space/enter folds a project"},
		{"r / t", "rename / set tags (stored in ~/.agentdeck, agent files are never modified)"},
		{"p", "pin / unpin — pinned sessions sit in a ★ Pinned section on top of every list"},
		{"+", "add the session (or all marked ones) to a group — type an existing name or a new one; each group is a tab"},
		{"- / E / X", "on a group's tab: remove from the group / rename the group / delete the group (sessions stay)"},
		{"a", "archive (hide) or restore — never deletes anything"},
		{"x", "stop a running session (claude stop for bg jobs, SIGTERM otherwise) — asks first"},
		{"y", "copy the resume command to the clipboard"},
		{"m / c", "mark sessions (✓), then c = share context: AI handoff note → new session (any agent), a running session, or clipboard"},
		{"[ ]  { }", "scroll the conversation preview back / forward"},
		{"mouse", "wheel over the list moves, over the preview scrolls the conversation; click selects (hold ⌥ to select text)"},
		{"h", "show/hide Codex subagent threads"},
		{"s", "stats: tokens per day, per agent, top projects"},
		{"R", "rescan session files now (also happens automatically)"},
		{"q", "quit"},
	}
	var lines []string
	lines = append(lines, stTitle.Render("◆ agentdeck — keys"), "")
	for _, r := range rows {
		lines = append(lines, key(fmt.Sprintf("%-11s", r[0]))+" "+r[1])
	}
	lines = append(lines, "", stDim.Render("Status: ")+stGreen.Render("● working")+"  "+stYellow.Render("◐ idle / waiting for you")+"  "+
		stAccent.Render("$ shell mode")+"  "+stDim.Render("★ pinned"))
	lines = append(lines, stDim.Render("Agents: ")+agentBadge(session.Claude)+" Claude Code  "+agentBadge(session.Codex)+" Codex  "+agentBadge(session.Pi)+" Pi")
	return stPaneOn.Width(m.w-2).Height(m.bodyHeight()-2).Padding(0, 1).Render(strings.Join(lines, "\n"))
}
