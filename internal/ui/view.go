package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/SrLeet03/agentdeck/internal/live"
	"github.com/SrLeet03/agentdeck/internal/session"
	"github.com/SrLeet03/agentdeck/internal/term"
	"github.com/charmbracelet/lipgloss"
)

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m Model) spin() string { return spinner[m.frame%len(spinner)] }

func (m Model) listWidth() int  { return max(46, m.w*55/100) }
func (m Model) bodyHeight() int { return max(3, m.h-3-m.stripHeight()) } // header + strip + tabs + footer
func (m Model) listHeight() int { return m.bodyHeight() - 2 }

// stripHeight is the height of the "now running" strip: shown when something runs and there's room.
func (m Model) stripHeight() int {
	if len(m.liveBy) == 0 || m.h < 26 || m.mode == modeStats || !m.loaded {
		return 0
	}
	return 4
}

func (m Model) View() string {
	if m.w == 0 {
		return ""
	}
	if m.w < 60 || m.h < 12 {
		return "agentdeck needs at least 60×12 — enlarge the window"
	}
	parts := []string{m.viewHeader()}
	if m.stripHeight() > 0 {
		parts = append(parts, m.viewStrip())
	}
	parts = append(parts, m.viewTabs())
	switch {
	case !m.loaded:
		parts = append(parts, m.viewSplash(m.w, m.bodyHeight()))
	case m.mode == modeStats:
		parts = append(parts, m.viewStats())
	default:
		lw := m.listWidth()
		parts = append(parts, lipgloss.JoinHorizontal(lipgloss.Top, m.viewList(lw), m.viewPreview(m.w-lw)))
	}
	parts = append(parts, m.viewFooter())
	frame := fitScreen(lipgloss.JoinVertical(lipgloss.Left, parts...), m.w, m.h)
	if box, ok := m.viewModal(); ok {
		frame = fitScreen(overlay(frame, box, m.w, m.h, true), m.w, m.h)
	}
	return frame
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

// spread places left and right on one line of width w.
func spread(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return trunc(left, w)
	}
	return left + strings.Repeat(" ", gap) + right
}

// ---- header ---------------------------------------------------------------------

func greeting(now time.Time) string {
	switch h := now.Hour(); {
	case h < 5:
		return "burning the midnight oil"
	case h < 12:
		return "good morning"
	case h < 17:
		return "good afternoon"
	case h < 22:
		return "good evening"
	}
	return "late-night hacking"
}

func (m Model) viewHeader() string {
	phase := 0.0
	if !m.loaded || m.busy != "" {
		phase = float64(m.frame) * 0.04
	}
	left := " " + gradient("◆ agentdeck", colAccent, colAccent2, true, phase)
	g := greeting(time.Now())
	if m.greet != "" {
		g += ", " + m.greet
	}
	left += "   " + stDim.Render(g)

	working, idle := 0, 0
	for _, p := range m.liveBy {
		if p.Status == live.Working {
			working++
		} else {
			idle++
		}
	}
	var right []string
	if m.loading && m.loaded {
		right = append(right, stDim.Render(m.spin()+" syncing"))
	}
	if working > 0 {
		right = append(right, stGreen.Render(fmt.Sprintf("%s %d working", m.spin(), working)))
	}
	if idle > 0 {
		right = append(right, stYellow.Render(fmt.Sprintf("◐ %d waiting", idle)))
	}
	days := m.dailyTotals(7)
	right = append(right, sparkline(days, colAccent, colAccent2)+" "+stBold.Render(humanTokens(days[len(days)-1]))+stDim.Render(" tokens today"))
	return spread(left, strings.Join(right, stFaint.Render("  │  "))+" ", m.w)
}

// dailyTotals returns token totals for the last n days (oldest first).
func (m Model) dailyTotals(n int) []int64 {
	out := make([]int64, n)
	keys := make([]string, n)
	for i := range keys {
		keys[i] = time.Now().AddDate(0, 0, -(n - 1 - i)).Format("2006-01-02")
	}
	for _, s := range m.sessions {
		for i, k := range keys {
			out[i] += s.Daily[k]
		}
	}
	return out
}

const sparkChars = "▁▂▃▄▅▆▇█"

// sparkline draws values as block bars colored along from→to.
func sparkline(vals []int64, from, to lipgloss.Color) string {
	var hi int64
	for _, v := range vals {
		hi = max(hi, v)
	}
	bars := []rune(sparkChars)
	var b strings.Builder
	for i, v := range vals {
		if hi == 0 || v == 0 {
			b.WriteString(stFaint.Render("▁"))
			continue
		}
		idx := int(float64(v) / float64(hi) * float64(len(bars)-1))
		c := blend(from, to, float64(i)/float64(max(1, len(vals)-1)))
		b.WriteString(lipgloss.NewStyle().Foreground(c).Render(string(bars[idx])))
	}
	return b.String()
}

// ---- now running strip -------------------------------------------------------------

type liveItem struct {
	s *session.Session
	p *live.Proc
}

func (m Model) liveItems() []liveItem {
	var items []liveItem
	for _, s := range append(append([]*session.Session{}, m.orphans...), m.sessions...) {
		if p := m.liveBy[s.Key()]; p != nil {
			items = append(items, liveItem{s, p})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		wi, wj := items[i].p.Status == live.Working, items[j].p.Status == live.Working
		if wi != wj {
			return wi
		}
		return m.activity(items[i].s).After(m.activity(items[j].s))
	})
	return items
}

func agentLabel(a session.Agent) string {
	switch a {
	case session.Claude:
		return "CLAUDE"
	case session.Codex:
		return "CODEX"
	}
	return "PI"
}

func (m Model) statusText(p *live.Proc, s *session.Session) string {
	switch p.Status {
	case live.Working:
		return stGreen.Render(m.spin() + " working")
	case live.Idle:
		return stYellow.Render("◐ waiting " + ago(m.activity(s)))
	case live.Shell:
		return stAccent.Render("$ shell")
	}
	return stDim.Render("○ running")
}

func (m Model) viewStrip() string {
	items := m.liveItems()
	iw := m.w - 4
	cardW := 38
	n := max(1, iw/cardW)
	if len(items) <= n {
		cardW = iw / max(1, len(items))
		cardW = min(cardW, 48)
	}
	var top, bottom []string
	sel := m.selected()
	shown := min(len(items), n)
	if len(items) > n {
		shown = max(1, (iw-6)/cardW)
	}
	for _, it := range items[:shown] {
		glyph := statusGlyph(it.p)
		if it.p.Status == live.Working {
			glyph = stGreen.Render(m.spin())
		}
		titleSt := stText
		if sel != nil && sel.Key() == it.s.Key() {
			titleSt = stAccent2.Bold(true)
		}
		label := lipgloss.NewStyle().Bold(true).Foreground(agentColor(it.s.Agent)).Render(agentLabel(it.s.Agent))
		l1 := glyph + " " + label + " " + titleSt.Render(oneLine(m.displayTitle(it.s)))
		l2 := "  " + stDim.Render(oneLine(it.s.ProjectBase())) + stFaint.Render(" · ") + m.statusText(it.p, it.s)
		top = append(top, pad(l1, cardW-2)+"  ")
		bottom = append(bottom, pad(l2, cardW-2)+"  ")
	}
	l1, l2 := strings.Join(top, ""), strings.Join(bottom, "")
	if more := len(items) - shown; more > 0 {
		l1 += stAccent.Render(fmt.Sprintf("+%d", more))
		l2 += stDim.Render("more")
	}
	title := " NOW RUNNING · " + fmt.Sprint(len(items)) + " "
	b := lipgloss.NewStyle().Foreground(colBorder)
	topLine := b.Render("╭─") + gradient(title, colAccent, colAccent2, true, 0) + b.Render(strings.Repeat("─", max(0, m.w-4-lipgloss.Width(title)))+"─╮")
	mid := func(s string) string { return b.Render("│ ") + pad(s, iw) + b.Render(" │") }
	return strings.Join([]string{topLine, mid(l1), mid(l2), b.Render("╰" + strings.Repeat("─", m.w-2) + "╯")}, "\n")
}

// ---- tabs -------------------------------------------------------------------------

func (m Model) tabCount(t tab) int {
	if int(t) < len(m.counts) {
		return m.counts[t]
	}
	return 0
}

func (m Model) viewTabs() string {
	var parts []string
	names := append(append([]string{}, tabNames...), m.store.Groups...)
	icons := map[int]string{int(tabPinned): "★ ", int(tabLive): "● "}
	for i, name := range names {
		isGroup := tab(i) >= tabFirstGroup
		icon := icons[i]
		if isGroup {
			icon = "▸"
		}
		count := m.tabCount(tab(i))
		if tab(i) == m.tab {
			parts = append(parts, stTabOn.Render(fmt.Sprintf("%s%s %d", icon, name, count)))
			continue
		}
		nameSt := stDim
		if isGroup {
			nameSt = stAccent2
		}
		parts = append(parts, " "+nameSt.Render(icon+name)+" "+stFaint.Render(fmt.Sprint(count))+" ")
	}
	first := 0
	for first < int(m.tab) && lipgloss.Width(strings.Join(parts[first:int(m.tab)+1], "")) > m.w-34 {
		first++
	}
	left := " " + strings.Join(parts[first:], "")
	if first > 0 {
		left = stDim.Render(" ‹") + strings.Join(parts[first:], "")
	}
	var right string
	switch {
	case m.mode == modeSearch:
		right = m.search.View()
	case m.query != "":
		right = stAccent.Render("/ "+m.query) + stDim.Render("  esc clears")
	default:
		right = stFaint.Render("/ ") + stDim.Render("search") + stFaint.Render("   ⌃K ") + stDim.Render("commands")
	}
	if m.grouped {
		right = stAccent2.Render("▦ by project  ") + right
	}
	return spread(left, right+" ", m.w)
}

// ---- footer -------------------------------------------------------------------------

func key(k string) string { return stKey.Render(k) }

func hints(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, key(pairs[i])+" "+stDim.Render(pairs[i+1]))
	}
	return strings.Join(parts, "   ")
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func (m Model) viewFooter() string {
	var left string
	switch {
	case m.mode == modePickTarget:
		left = pill("SEND CONTEXT", colAccent) + " " + stText.Render("move to a running session (●/◐) and press ") + key("enter") + stDim.Render("  ·  esc cancel")
	case m.mode == modeStats:
		left = hints("esc", "back", "T", "theme", "q", "quit")
	case m.mode != modeNormal && m.mode != modeSearch:
		left = hints("esc", "close")
	case len(m.marked) > 0:
		left = pill(fmt.Sprintf("✓ %d MARKED", len(m.marked)), colAccent2) + "  " + hints("c", "share context", "+", "add to group", "m", "toggle", "esc", "clear")
	default:
		if _, ok := m.currentGroup(); ok {
			left = hints("enter", "open", "+", "add", "-", "remove", "E", "rename group", "X", "delete group", "⌃K", "commands", "?", "help")
		} else {
			left = hints("enter", "open", "n", "new", "⌃K", "commands", "/", "search", "p", "pin", "+", "group", "c", "share", "T", "theme", "?", "help")
		}
	}
	return spread(" "+left, m.viewToast()+" ", m.w)
}

// viewToast is the right side of the footer: a sliding notification, the background job, or nothing.
func (m Model) viewToast() string {
	if m.toastActive() {
		text := m.status
		if r := []rune(text); len(r) > m.w/2 {
			text = string(r[:m.w/2-1]) + "…"
		}
		// Slide in: reveal a few more characters each frame.
		shown := int(time.Since(m.statusAt)/animEvery)*6 + 4
		if r := []rune(text); shown < len(r) {
			text = string(r[:shown])
		}
		if m.statusErr {
			return pill("✗ "+text, colRed)
		}
		return pill("✓ "+text, colGreen)
	}
	if m.busy != "" {
		return stAccent.Render(m.spin()+" ") + gradient(m.busy+"…", colAccent, colAccent2, false, float64(m.frame)*0.05) +
			stDim.Render(fmt.Sprintf(" %ds", int(time.Since(m.busySince).Seconds())))
	}
	return ""
}

// ---- list ---------------------------------------------------------------------------

func (m Model) viewList(w int) string {
	iw := w - 2
	cw := iw - 1 // last column is the scrollbar
	h := m.listHeight()
	var lines []string
	if len(m.rows) == 0 {
		lines = m.emptyState(cw, h)
	}
	for i := m.offset; i < len(m.rows) && len(lines) < h; i++ {
		lines = append(lines, m.renderRow(m.rows[i], cw, i == m.cursor)...)
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	bar := m.scrollbar(h)
	for i := range lines {
		lines[i] = pad(lines[i], cw) + bar[i]
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colBorder).Width(iw).Height(h).Render(strings.Join(lines, "\n"))
}

func (m Model) scrollbar(h int) []string {
	out := make([]string, h)
	total, before := 0, 0
	for i := range m.rows {
		if i == m.offset {
			before = total
		}
		total += m.rowHeight(i)
	}
	if total <= h {
		for i := range out {
			out[i] = " "
		}
		return out
	}
	size := max(1, h*h/total)
	pos := min(h-size, before*h/total)
	for i := range out {
		if i >= pos && i < pos+size {
			out[i] = stAccent.Render("┃")
		} else {
			out[i] = stFaint.Render("│")
		}
	}
	return out
}

func (m Model) emptyState(w, h int) []string {
	art := []string{"   ╭───╮", "   │ ◇ │", "   ╰───╯"}
	msg, sub := "nothing here yet", "start one with n, or switch tabs with tab"
	switch {
	case m.query != "":
		msg, sub = "no sessions match “"+m.query+"”", "try fewer letters, a #tag or an @group"
	case m.tab == tabLive:
		msg, sub = "nothing is running", "running Claude, Codex and Pi sessions appear here"
	case m.tab == tabPinned:
		msg, sub = "no pinned sessions", "press p on any session to pin it here"
	case m.tab == tabArchived:
		msg, sub = "archive is empty", "a hides a session; nothing is ever deleted"
	}
	var out []string
	for range max(0, h/2-4) {
		out = append(out, "")
	}
	center := func(s string) string { return strings.Repeat(" ", max(0, (w-lipgloss.Width(s))/2)) + s }
	for _, a := range art {
		out = append(out, center(gradient(a, colAccent, colAccent2, false, 0)))
	}
	out = append(out, "", center(stBold.Render(msg)), center(stDim.Render(sub)))
	return out
}

func (m Model) renderRow(r row, w int, sel bool) []string {
	bar := " "
	if sel {
		bar = stAccent.Render("▌")
	}
	if r.section {
		arrow := "▾"
		if m.collapsed[r.header] {
			arrow = "▸"
		}
		st := stDim.Bold(true)
		if r.header == secPinned {
			st = stYellow.Bold(true)
		}
		if sel {
			st = stAccent.Bold(true)
		}
		label := st.Render(arrow+" "+strings.ToUpper(r.header)) + "  " + stFaint.Render(fmt.Sprint(r.count))
		return []string{bar + label + " " + stFaint.Render(strings.Repeat("─", max(0, w-lipgloss.Width(label)-2)))}
	}
	if r.header != "" {
		arrow := "▾"
		if m.collapsed[r.header] {
			arrow = "▸"
		}
		st := stAccent.Bold(true)
		if sel {
			st = stAccent2.Bold(true)
		}
		label := st.Render(arrow+" "+session.ShortPath(r.header)) + "  " + stDim.Render(fmt.Sprintf("%d · %s", r.count, ago(r.latest)))
		return []string{bar + label + " " + stFaint.Render(strings.Repeat("─", max(0, w-lipgloss.Width(label)-2)))}
	}

	s := r.s
	meta := m.store.Get(s.Key())
	p := m.liveBy[s.Key()]
	indent := ""
	if m.grouped {
		indent = "  "
	}

	// Line 1: status, pin/mark, title (with search highlights), age.
	glyph := statusGlyph(p)
	if p != nil && p.Status == live.Working {
		glyph = stGreen.Render(m.spin())
	}
	pin := " "
	switch {
	case m.marked[s.Key()]:
		pin = stAccent2.Bold(true).Render("✓")
	case meta.Pinned:
		pin = stYellow.Render("★")
	}
	titleSt := stText
	switch {
	case sel:
		titleSt = stBold
	case s.Subagent, p == nil && time.Since(m.activity(s)) > 7*24*time.Hour:
		titleSt = stDim
	}
	when := ago(m.activity(s))
	whenR := stDim.Render(when)
	if sel {
		whenR = stText.Render(when)
	}
	titleW := max(8, w-len(indent)-6-lipgloss.Width(when)-1)
	title := oneLine(m.displayTitle(s))
	if r := []rune(title); len(r) > titleW {
		title = string(r[:titleW-1]) + "…"
	}
	titleR := highlight(title, m.match[s.Key()].pos, titleSt, stAccent2.Bold(true).Underline(true))
	l1 := spread(bar+indent+glyph+" "+pin+" "+titleR, whenR, w)

	// Line 2: agent, project, branch, tags/groups … activity sparkline, tokens.
	agentSt := lipgloss.NewStyle().Foreground(agentColor(s.Agent))
	if sel {
		agentSt = agentSt.Bold(true)
	}
	meta2 := agentSt.Render(fmt.Sprintf("%-6s", agentLabel(s.Agent)))
	proj := oneLine(s.ProjectBase())
	if m.grouped {
		proj = oneLine(worktreeName(s.Cwd))
	}
	if proj != "" {
		meta2 += "  " + stDim.Render(proj)
	}
	if s.Branch != "" && s.Branch != "HEAD" {
		br := oneLine(s.Branch)
		if r := []rune(br); len(r) > 22 {
			br = string(r[:21]) + "…"
		}
		meta2 += "  " + stFaint.Render("⎇ ") + stDim.Render(br)
	}
	for _, t := range meta.Tags {
		meta2 += " " + stAccent.Render("#"+t)
	}
	for _, g := range meta.Groups {
		meta2 += " " + stAccent2.Render("▸"+g)
	}
	right := ""
	if days := sessionDays(s, 7); hasAny(days) {
		right = sparkline(days, blend(agentColor(s.Agent), colFaint, 0.5), agentColor(s.Agent)) + " "
	}
	if tok := s.Tokens.Total(); tok > 0 {
		right += stDim.Render(humanTokens(tok))
	}
	l2 := spread(bar+indent+"    "+meta2, right, w)
	return []string{l1, l2}
}

func sessionDays(s *session.Session, n int) []int64 {
	out := make([]int64, n)
	for i := range out {
		out[i] = s.Daily[time.Now().AddDate(0, 0, -(n-1-i)).Format("2006-01-02")]
	}
	return out
}

func hasAny(v []int64) bool {
	for _, x := range v {
		if x > 0 {
			return true
		}
	}
	return false
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

// ---- preview -----------------------------------------------------------------------

func (m Model) viewPreview(w int) string {
	iw := w - 4 // border + 1 col padding each side
	h := m.bodyHeight() - 2
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colBorder).Width(w-2).Height(h).Padding(0, 1)
	r := m.cur()
	if r == nil {
		return style.Render(stDim.Render("nothing selected"))
	}
	if r.section {
		txt := stBold.Render(r.header) + "\n\n" + stDim.Render(plural(r.count, "session")+" · enter/space folds this section")
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
	status := ""
	if p != nil {
		switch p.Status {
		case live.Working:
			status = pill(m.spin()+" WORKING", colGreen)
		case live.Idle:
			status = pill("◐ WAITING FOR YOU", colYellow)
		case live.Shell:
			status = pill("$ SHELL", colAccent)
		default:
			status = pill("○ RUNNING", colDim)
		}
	}
	head = append(head, spread(pill(strings.ToUpper(agentName(s.Agent)), agentColor(s.Agent))+" "+stDim.Render(oneLine(s.Source)), status, iw))
	head = append(head, "")
	for _, l := range clipPlain(wrapPlain(oneLine(m.displayTitle(s)), iw), 3) {
		head = append(head, stBold.Render(l))
	}
	if meta.Name != "" && oneLine(s.DisplayTitle()) != meta.Name {
		head = append(head, stDim.Render(trunc("originally: "+oneLine(s.DisplayTitle()), iw)))
	}
	head = append(head, "")
	chips := stFaint.Render("⌂ ") + stText.Render(oneLine(session.ShortPath(s.Cwd)))
	if s.Branch != "" && s.Branch != "HEAD" {
		chips += "   " + stFaint.Render("⎇ ") + stText.Render(oneLine(s.Branch))
	}
	head = append(head, trunc(chips, iw))
	if s.Model != "" {
		head = append(head, trunc(stFaint.Render("◇ ")+stDim.Render(oneLine(s.Model)), iw))
	}
	rel := ago(m.activity(s))
	if rel == "now" {
		rel = "just now"
	} else if !strings.Contains(rel, " ") {
		rel += " ago"
	}
	head = append(head, trunc(stFaint.Render("◷ ")+stDim.Render("started "+stamp(s.Created)+" · active ")+stText.Render(rel), iw))
	msgs := fmt.Sprintf("%d from you · %d from the agent", s.UserMsgs, s.AgentMsgs)
	if s.Subagents > 0 {
		msgs += fmt.Sprintf(" · %d subagents", s.Subagents)
	}
	head = append(head, trunc(stFaint.Render("✉ ")+stDim.Render(msgs), iw))
	head = append(head, m.tokenLine(s.Tokens, iw))
	if len(meta.Tags)+len(meta.Groups) > 0 {
		var c []string
		for _, t := range meta.Tags {
			c = append(c, stAccent.Render("#"+t))
		}
		for _, g := range meta.Groups {
			c = append(c, stAccent2.Render("▸"+g))
		}
		head = append(head, trunc(strings.Join(c, "  "), iw))
	}
	if p != nil {
		info := fmt.Sprintf("pid %d", p.PID)
		switch {
		case p.Agent == "claude" && p.Kind == "bg":
			info += " · background job " + p.JobID
			if p.AttachedTTY != "" {
				info += " · attached on " + p.AttachedTTY
			}
		case p.TTY != "":
			info += " · " + p.TTY
		}
		head = append(head, trunc(stFaint.Render("⚡ ")+stDim.Render(info), iw))
	}
	if _, exact := term.ExistingDir(s.Cwd); !exact && s.Cwd != "" {
		head = append(head, stYellow.Render(trunc("⚠ folder no longer exists (deleted worktree?)", iw)))
	}
	if s.Path != "" {
		head = append(head, trunc(stFaint.Render("$ "+baseResumeCommand(s)), iw))
	}

	pd := m.preview[s.Key()]
	label := " conversation "
	if pd != nil && pd.truncated {
		label = " conversation · latest part "
	}
	if m.pScroll > 0 {
		label += fmt.Sprintf("↑%d ", m.pScroll)
	}
	side := max(0, (iw-lipgloss.Width(label))/2)
	head = append(head, "", stFaint.Render(strings.Repeat("─", side))+stDim.Render(label)+stFaint.Render(strings.Repeat("─", max(0, iw-side-lipgloss.Width(label)))))

	var conv []string
	switch {
	case s.Path == "":
		conv = []string{stDim.Render("no transcript yet")}
	case pd == nil:
		conv = []string{"", stDim.Render(m.spin() + " loading conversation…")}
	default:
		conv = m.conversationLines(s, pd, iw)
		if len(conv) == 0 {
			conv = []string{stDim.Render("no messages")}
		}
	}
	if len(head) > h {
		head = head[:h]
	}
	convH := max(0, h-len(head))
	endIdx := len(conv) - min(m.pScroll, max(0, len(conv)-convH))
	start := max(0, endIdx-convH)
	return style.Render(strings.Join(append(head, conv[start:endIdx]...), "\n"))
}

// tokenLine shows the token total with a stacked bar of input / output / cache read / cache write.
func (m Model) tokenLine(t session.Tokens, w int) string {
	total := t.Total()
	line := stFaint.Render("Σ ") + stBold.Render(humanTokens(total)) + stDim.Render(" tokens ")
	if total == 0 {
		return line
	}
	barW := min(24, max(8, w-lipgloss.Width(line)-34))
	parts := []struct {
		n int64
		c lipgloss.Color
	}{{t.In, colAccent}, {t.Out, colAccent2}, {t.CacheRead, colCode}, {t.CacheWrite, colDim}}
	var bar strings.Builder
	used := 0
	for i, p := range parts {
		n := int(float64(p.n) / float64(total) * float64(barW))
		if p.n > 0 && n == 0 {
			n = 1
		}
		if i == len(parts)-1 {
			n = max(0, barW-used)
		}
		used += n
		bar.WriteString(lipgloss.NewStyle().Foreground(p.c).Render(strings.Repeat("█", n)))
	}
	legend := stAccent.Render("in ") + stDim.Render(humanTokens(t.In)) + " " + stAccent2.Render("out ") + stDim.Render(humanTokens(t.Out)) +
		" " + stCode.Render("cache ") + stDim.Render(humanTokens(t.CacheRead+t.CacheWrite))
	return trunc(line+bar.String()+" "+legend, w)
}

// conversationLines renders (and caches) the conversation as chat: your messages with an accent bar,
// agent replies as markdown, and consecutive tool calls folded into one line.
func (m Model) conversationLines(s *session.Session, pd *previewData, w int) []string {
	if c, ok := m.conv[s.Key()]; ok && c.width == w && c.theme == theme.Name && c.mtime.Equal(pd.mtime) {
		return c.lines
	}
	var out []string
	agentSt := lipgloss.NewStyle().Foreground(agentColor(s.Agent)).Bold(true)
	var tools []session.Message
	flushTools := func() {
		if len(tools) == 0 {
			return
		}
		var names []string
		counts := map[string]int{}
		for _, t := range tools {
			name := strings.Fields(t.Text + " ?")[0]
			if counts[name] == 0 {
				names = append(names, name)
			}
			counts[name]++
		}
		var parts []string
		for _, n := range names {
			if counts[n] > 1 {
				parts = append(parts, fmt.Sprintf("%s ×%d", n, counts[n]))
			} else {
				parts = append(parts, n)
			}
		}
		last := oneLine(tools[len(tools)-1].Text)
		if i := strings.IndexByte(last, ' '); i >= 0 {
			last = last[i+1:]
		}
		line := stFaint.Render("  ⚙ ") + stDim.Render(strings.Join(parts, " · "))
		if last != "" {
			line += stFaint.Render("  ") + stFaint.Render(last)
		}
		out = append(out, trunc(line, w))
		tools = nil
	}
	for _, msg := range m.previewMsgs(pd) {
		switch msg.Role {
		case "tool":
			tools = append(tools, msg)
			continue
		case "user":
			flushTools()
			out = append(out, "", stAccent2.Bold(true).Render("you")+stFaint.Render(" · "+clock(msg.At)))
			for _, l := range clip(renderMarkdown(msg.Text, w-2, stText), 14) {
				out = append(out, stAccent2.Render("▎")+" "+l)
			}
		case "assistant":
			flushTools()
			out = append(out, "", agentSt.Render(strings.ToLower(agentLabel(s.Agent)))+stFaint.Render(" · "+clock(msg.At)))
			out = append(out, clip(renderMarkdown(msg.Text, w, stText), 18)...)
		}
	}
	flushTools()
	m.conv[s.Key()] = convEntry{width: w, theme: theme.Name, mtime: pd.mtime, lines: out}
	return out
}

// previewMsgs keeps the conversation tail bounded so rendering stays fast on huge sessions.
func (m Model) previewMsgs(pd *previewData) []session.Message {
	const keep = 400
	if len(pd.msgs) > keep {
		return pd.msgs[len(pd.msgs)-keep:]
	}
	return pd.msgs
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

func clip(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return append(lines[:n:n], stDim.Render(fmt.Sprintf("… %d more lines", len(lines)-n)))
}

func clipPlain(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	l := append([]string{}, lines[:n]...)
	l[n-1] = strings.TrimRight(l[n-1], " ") + "…"
	return l
}

func (m Model) groupSummary(project string, w int) string {
	var n, running int
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
			running++
		}
		if a := m.activity(s); a.After(latest) {
			latest = a
		}
	}
	lines := []string{pill("PROJECT", colAccent), "", stBold.Render(session.ShortPath(project)), ""}
	lines = append(lines, stDim.Render("sessions  ")+stText.Render(fmt.Sprint(n))+"   "+
		lipgloss.NewStyle().Foreground(colClaude).Render(fmt.Sprintf("CLAUDE %d", agents[session.Claude]))+"  "+
		lipgloss.NewStyle().Foreground(colCodex).Render(fmt.Sprintf("CODEX %d", agents[session.Codex]))+"  "+
		lipgloss.NewStyle().Foreground(colPi).Render(fmt.Sprintf("PI %d", agents[session.Pi])))
	lines = append(lines, stDim.Render("running   ")+stText.Render(fmt.Sprint(running)))
	lines = append(lines, stDim.Render("tokens    ")+stText.Render(humanTokens(tok)))
	lines = append(lines, stDim.Render("active    ")+stText.Render(stamp(latest)))
	lines = append(lines, "", hints("enter", "fold", "n", "new session here"))
	for i := range lines {
		lines[i] = trunc(lines[i], w)
	}
	return strings.Join(lines, "\n")
}

// ---- splash ----------------------------------------------------------------------

func (m Model) viewSplash(w, h int) string {
	logo := []string{"█▀▄ █▀▀ █▀▀ █▄▀", "█▄▀ ██▄ █▄▄ █ █"}
	phase := float64(m.frame) * 0.04
	var lines []string
	for range max(0, h/2-4) {
		lines = append(lines, "")
	}
	center := func(s string) string { return strings.Repeat(" ", max(0, (w-lipgloss.Width(s))/2)) + s }
	for _, l := range logo {
		lines = append(lines, center(gradient(l, colAccent, colAccent2, true, phase)))
	}
	lines = append(lines, "", center(stAccent.Render(m.spin())+" "+stText.Render("reading your Claude, Codex & Pi sessions")),
		center(stDim.Render("the first run indexes everything — next time it's instant")))
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines[:h], "\n")
}

// ---- modals ------------------------------------------------------------------------

func (m Model) modalWidth(want int) int { return min(want, m.w-6) }

func modalTitle(s string) string { return gradient(s, colAccent, colAccent2, true, 0) }

func option(k, label, desc string) string {
	line := key(" "+k+" ") + "  " + stBold.Render(label)
	if desc != "" {
		line += "\n     " + stDim.Render(desc)
	}
	return line
}

func (m Model) viewModal() (string, bool) {
	w := m.modalWidth(64)
	box := func(lines ...string) (string, bool) {
		return stModal.Width(w - 2).Render(strings.Join(lines, "\n")), true
	}
	switch m.mode {
	case modePalette:
		return m.viewPalette(), true
	case modeHelp:
		return m.viewHelp(), true
	case modeConfirmStop:
		what := "this session"
		if s := m.selected(); s != nil {
			what = "“" + trunc(oneLine(m.displayTitle(s)), w-14) + "”"
			if p := m.liveBy[s.Key()]; p != nil && p.Agent == "claude" && p.Kind == "bg" {
				what += "\n" + stDim.Render("background Claude job "+p.JobID+" — uses `claude stop`")
			} else if p != nil {
				what += "\n" + stDim.Render(fmt.Sprintf("%s pid %d — sends SIGTERM", p.Agent, p.PID))
			}
		}
		return box(stRed.Bold(true).Render("■ Stop running session?"), "", what, "", pill(" y  stop ", colRed)+"   "+stDim.Render("any other key cancels"))
	case modeConfirmGroupDelete:
		g, _ := m.currentGroup()
		return box(stRed.Bold(true).Render("■ Delete group ▸"+g+"?"), "", stDim.Render("Only the group goes — its sessions stay where they are."), "",
			pill(" y  delete ", colRed)+"   "+stDim.Render("any other key cancels"))
	case modeShareMenu:
		return box(modalTitle("⇄ Share context"), stDim.Render("from "+plural(len(m.share), "session")+" — an AI handoff note with links to the full transcripts"), "",
			option("n", "Start a new session with it", "Claude Code, Codex or Pi, in any folder"), "",
			option("s", "Send to a running session", "types a “read this handoff” line into its tab"), "",
			option("y", "Copy to clipboard", "paste it anywhere"), "", stDim.Render("esc cancel"))
	case modeShareAgent, modeNewAgent:
		title := "✦ New session"
		if m.mode == modeShareAgent {
			title = "✦ New session with context"
		}
		ag := func(k string, a session.Agent, desc string) string {
			return key(" "+k+" ") + "  " + lipgloss.NewStyle().Bold(true).Foreground(agentColor(a)).Render(agentName(a)) + "  " + stDim.Render(desc)
		}
		return box(modalTitle(title), "", ag("c", session.Claude, "Anthropic"), "", ag("x", session.Codex, "OpenAI"), "", ag("p", session.Pi, "any model"), "", stDim.Render("esc cancel"))
	case modeRename, modeTags, modeNewDir, modeShareDir, modeGroupAdd, modeGroupRename:
		icon := map[mode]string{modeRename: "✎ ", modeTags: "# ", modeNewDir: "⌂ ", modeShareDir: "⌂ ", modeGroupAdd: "▸ ", modeGroupRename: "✎ "}[m.mode]
		in := m.input
		in.Width = w - 10
		return box(modalTitle(icon+m.inputTitle), "", in.View(), "", stDim.Render("enter save · esc cancel"))
	}
	return "", false
}

// ---- help --------------------------------------------------------------------------

func (m Model) viewHelp() string {
	enter := "running → jump to its tab · otherwise resume in a new " + term.Name() + " tab"
	if term.Name() == "clipboard" {
		enter = "copy the session's resume command (no supported terminal — set \"terminal\" in config.json)"
	}
	rows := [][2]string{
		{"↑↓ j k", "move · pgup/pgdn · home/end · mouse wheel & click"},
		{"tab 1-9", "switch tab: All, Pinned, Live, Claude, Codex, Pi, Archived, your ▸groups"},
		{"enter", enter},
		{"⌃K", "command palette: every action, tab, theme and session"},
		{"n", "new session — pick the agent, then the folder"},
		{"/", "fuzzy search · #tag · @group · esc clears"},
		{"g", "group by project (worktrees fold into their repo)"},
		{"p", "pin — pinned sessions sit on top of every list"},
		{"+  -", "add to a group · remove from it (on the group's tab)"},
		{"E  X", "rename · delete the current group"},
		{"r  t", "rename · tag (agent files are never modified)"},
		{"m  c", "mark sessions · share their context (AI handoff + transcript links)"},
		{"a", "archive / restore — nothing is ever deleted"},
		{"x", "stop a running session (asks first)"},
		{"y", "copy the resume command"},
		{"[ ]  { }", "scroll the conversation preview"},
		{"h", "show / hide Codex subagent threads"},
		{"s", "stats · T theme · R rescan · q quit"},
	}
	var lines []string
	lines = append(lines, modalTitle("◆ agentdeck — keys"), "")
	for _, r := range rows {
		lines = append(lines, key(fmt.Sprintf("%-9s", r[0]))+"  "+stText.Render(r[1]))
	}
	lines = append(lines, "", stGreen.Render(spinner[0]+" working")+"   "+stYellow.Render("◐ waiting for you")+"   "+stAccent.Render("$ shell")+"   "+
		stYellow.Render("★")+stDim.Render(" pinned")+"   "+stAccent2.Render("✓")+stDim.Render(" marked"))
	lines = append(lines, "", stDim.Render("theme ")+stText.Render(theme.Name)+stDim.Render(" · terminal ")+stText.Render(term.Name())+stDim.Render(" · esc close"))
	return stModal.Width(m.modalWidth(92) - 2).Render(strings.Join(lines, "\n"))
}
