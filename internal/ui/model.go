// Package ui is the Bubble Tea dashboard.
package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/SrLeet03/agentdeck/internal/config"
	"github.com/SrLeet03/agentdeck/internal/handoff"
	"github.com/SrLeet03/agentdeck/internal/live"
	"github.com/SrLeet03/agentdeck/internal/session"
	"github.com/SrLeet03/agentdeck/internal/state"
	"github.com/SrLeet03/agentdeck/internal/term"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type tab int

const (
	tabAll tab = iota
	tabPinned
	tabLive
	tabClaude
	tabCodex
	tabPi
	tabArchived
	tabFirstGroup // user groups follow the built-in tabs
)

var tabNames = []string{"All", "Pinned", "Live", "Claude", "Codex", "Pi", "Archived"}

const (
	secPinned = "★ Pinned"
	secRecent = "Recent"
)

type mode int

const (
	modeNormal mode = iota
	modeSearch
	modeRename
	modeTags
	modeConfirmStop
	modeNewAgent
	modeNewDir
	modeHelp
	modeStats
	modeShareMenu
	modeShareAgent
	modeShareDir
	modePickTarget
	modeGroupAdd
	modeGroupRename
	modeConfirmGroupDelete
)

type shareAction int

const (
	shareNew shareAction = iota
	shareSend
	shareCopy
)

const (
	tickEvery     = 2 * time.Second
	reloadEveryN  = 8                // ticks between incremental rescans of session files
	workingWindow = 12 * time.Second // codex/pi count as "working" if their log changed this recently
)

type row struct {
	header  string // non-empty for project group headers and list sections
	section bool   // header is a ★ Pinned / Recent divider rather than a project
	count   int
	latest  time.Time
	s       *session.Session
}

type previewData struct {
	msgs      []session.Message
	truncated bool
	mtime     time.Time
}

type Model struct {
	loader  *session.Loader
	store   *state.Store
	loaded  bool
	loading bool

	sessions []*session.Session
	orphans  []*session.Session // live processes with no session file yet
	procs    []live.Proc
	liveBy   map[string]*live.Proc
	mtimes   map[string]time.Time // fresh mtimes of live session files

	tab       tab
	grouped   bool
	collapsed map[string]bool
	showSub   bool
	query     string
	search    textinput.Model
	input     textinput.Model
	mode      mode
	newAgent  session.Agent

	rows     []row
	cursor   int
	offset   int
	preview  map[string]*previewData
	inflight map[string]bool
	pScroll  int

	marked    map[string]bool
	share     []*session.Session // sources for the share flow in progress
	shareTab  tab                // tab to restore after picking a target
	shareAg   session.Agent
	busy      string // background handoff in progress
	busySince time.Time

	status    string
	statusErr bool
	statusAt  time.Time
	ticks     int
	w, h      int
}

type (
	loadedMsg  []*session.Session
	liveMsg    []live.Proc
	tickMsg    time.Time
	previewMsg struct {
		key       string
		msgs      []session.Message
		truncated bool
		mtime     time.Time
	}
	handoffMsg struct {
		res     handoff.Result
		err     error
		action  shareAction
		agent   session.Agent
		dir     string
		target  *live.Proc
		sources int
	}
	actionMsg struct {
		text   string
		err    error
		reload bool
	}
)

func New() Model {
	si := textinput.New()
	si.Prompt = "/ "
	si.Placeholder = "search title, prompt, project, #tag, model…"
	si.CharLimit = 200
	in := textinput.New()
	in.CharLimit = 300
	return Model{
		loader:    session.NewLoader(),
		store:     state.Open(session.DataDir()),
		collapsed: map[string]bool{},
		marked:    map[string]bool{},
		liveBy:    map[string]*live.Proc{},
		mtimes:    map[string]time.Time{},
		preview:   map[string]*previewData{},
		inflight:  map[string]bool{},
		search:    si,
		input:     in,
		loading:   true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadCmd(), liveCmd(), tick())
}

func (m Model) loadCmd() tea.Cmd {
	l := m.loader
	return func() tea.Msg {
		s := l.Load()
		l.Save()
		return loadedMsg(s)
	}
}

func liveCmd() tea.Cmd { return func() tea.Msg { return liveMsg(live.Scan()) } }

func tick() tea.Cmd { return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) }) }

func previewCmd(key string, s *session.Session) tea.Cmd {
	return func() tea.Msg {
		mt := session.ModTime(s.Path)
		msgs, tr := session.Messages(s)
		return previewMsg{key, msgs, tr, mt}
	}
}

func (m *Model) flash(text string, err bool) {
	m.status, m.statusErr, m.statusAt = text, err, time.Now()
}

// ---- update ----------------------------------------------------------------

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.search.Width = max(20, m.w/3)
		m.input.Width = max(20, m.w-30)
		m.clampScroll()
		return m, nil

	case loadedMsg:
		first := !m.loaded
		m.sessions = msg
		m.loaded, m.loading = true, false
		m.mapLive()
		if first {
			m.rows = nil // start at the top rather than wherever live-only rows put the cursor
			m.cursor, m.offset = 0, 0
		}
		m.rebuild()
		return m, m.ensurePreview(false)

	case liveMsg:
		m.procs = msg
		m.mapLive()
		m.rebuild()
		return m, m.ensurePreview(false)

	case tickMsg:
		m.ticks++
		cmds := []tea.Cmd{tick(), liveCmd()}
		if m.ticks%reloadEveryN == 0 && !m.loading {
			m.loading = true
			cmds = append(cmds, m.loadCmd())
		}
		return m, tea.Batch(cmds...)

	case previewMsg:
		delete(m.inflight, msg.key)
		m.preview[msg.key] = &previewData{msg.msgs, msg.truncated, msg.mtime}
		return m, nil

	case actionMsg:
		if msg.err != nil {
			m.flash(msg.err.Error(), true)
		} else if msg.text != "" {
			m.flash(msg.text, false)
		}
		if msg.reload {
			return m, liveCmd()
		}
		return m, nil

	case handoffMsg:
		m.busy = ""
		if msg.err != nil {
			m.flash("handoff failed: "+msg.err.Error(), true)
			return m, nil
		}
		return m, deliverHandoff(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}
	return m, nil
}

func (m Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := k.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.mode {
	case modeHelp, modeStats:
		switch key {
		case "q":
			return m, tea.Quit
		case "esc", "?", "s", "enter":
			m.mode = modeNormal
		}
		return m, nil

	case modeSearch:
		switch key {
		case "esc":
			m.mode = modeNormal
			m.search.Blur()
			m.search.SetValue("")
			m.query = ""
			m.rebuild()
			return m, m.ensurePreview(false)
		case "enter", "down", "up":
			m.mode = modeNormal
			m.search.Blur()
			return m, m.ensurePreview(false)
		}
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(k)
		if q := m.search.Value(); q != m.query {
			m.query = q
			m.cursor, m.offset = 0, 0
			m.rebuild()
		}
		return m, tea.Batch(cmd, m.ensurePreview(false))

	case modeRename, modeTags, modeNewDir, modeShareDir, modeGroupAdd, modeGroupRename:
		switch key {
		case "esc":
			m.mode = modeNormal
			m.input.Blur()
			m.share = nil
			return m, nil
		case "enter":
			return m.submitInput()
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(k)
		return m, cmd

	case modeNewAgent:
		m.mode = modeNormal
		switch key {
		case "c", "1":
			m.newAgent = session.Claude
		case "x", "2":
			m.newAgent = session.Codex
		case "p", "3":
			m.newAgent = session.Pi
		default:
			return m, nil
		}
		dir := "~"
		if s := m.selected(); s != nil && s.Cwd != "" {
			dir = session.ShortPath(s.Cwd)
		}
		m.openInput(modeNewDir, "Folder for new "+agentName(m.newAgent)+" session: ", dir)
		return m, textinput.Blink

	case modeShareMenu:
		m.mode = modeNormal
		switch key {
		case "n":
			m.mode = modeShareAgent
		case "s":
			m.shareTab = m.tab
			m.mode = modePickTarget
			m.query = "" // show every running session, not just ones matching the source search
			m.search.SetValue("")
			m.setTab(tabLive)
			m.flash("pick the running session to send the context to, then press enter", false)
		case "y":
			return m, m.startHandoff(shareCopy, "", "", nil)
		default:
			m.share = nil
		}
		return m, nil

	case modeShareAgent:
		m.mode = modeNormal
		switch key {
		case "c", "1":
			m.shareAg = session.Claude
		case "x", "2":
			m.shareAg = session.Codex
		case "p", "3":
			m.shareAg = session.Pi
		default:
			m.share = nil
			return m, nil
		}
		dir := "~"
		if len(m.share) > 0 && m.share[0].Cwd != "" {
			if d, _ := term.ExistingDir(m.share[0].Cwd); d != "" {
				dir = session.ShortPath(d)
			}
		}
		m.openInput(modeShareDir, "Folder for new "+agentName(m.shareAg)+" session with context: ", dir)
		return m, textinput.Blink

	case modePickTarget:
		switch key {
		case "esc":
			m.mode = modeNormal
			m.share = nil
			m.setTab(m.shareTab)
			m.flash("share cancelled", false)
			return m, nil
		case "enter":
			s := m.selected()
			if s == nil || m.liveBy[s.Key()] == nil {
				m.flash("that session isn't running — pick one with ●/◐", true)
				return m, nil
			}
			for _, src := range m.share {
				if src.Key() == s.Key() {
					m.flash("pick a different session than the source", true)
					return m, nil
				}
			}
			p := *m.liveBy[s.Key()]
			m.mode = modeNormal
			m.setTab(m.shareTab)
			return m, m.startHandoff(shareSend, "", "", &p)
		case "up", "k", "down", "j", "pgup", "pgdown", "ctrl+u", "ctrl+d", "home", "end", "tab", "shift+tab",
			"1", "2", "3", "4", "5", "6", "7":
			// fall through to normal navigation below
		default:
			return m, nil
		}

	case modeConfirmGroupDelete:
		m.mode = modeNormal
		if g, ok := m.currentGroup(); ok && (key == "y" || key == "Y") {
			m.store.DeleteGroup(g)
			m.setTab(tabAll)
			m.flash("deleted group "+g+" (sessions untouched)", false)
		} else {
			m.flash("cancelled", false)
		}
		return m, nil

	case modeConfirmStop:
		m.mode = modeNormal
		if key == "y" || key == "Y" {
			return m, m.stopSelected()
		}
		m.flash("stop cancelled", false)
		return m, nil
	}

	// normal mode
	if k.Type == tea.KeyRunes && len(k.Runes) > 1 && k.Runes[0] == '/' {
		// pasted or fast-typed "/query" arrives as one burst
		m.mode = modeSearch
		m.search.Focus()
		m.search.SetValue(string(k.Runes[1:]))
		m.search.CursorEnd()
		m.query = m.search.Value()
		m.cursor, m.offset = 0, 0
		m.rebuild()
		return m, tea.Batch(textinput.Blink, m.ensurePreview(false))
	}
	prev := m.cursor
	switch key {
	case "q":
		return m, tea.Quit
	case "esc":
		if m.query != "" {
			m.query = ""
			m.search.SetValue("")
			m.rebuild()
		} else if len(m.marked) > 0 {
			m.marked = map[string]bool{}
			m.flash("marks cleared", false)
		}
	case "m":
		if s := m.selected(); s != nil && s.Path != "" {
			if m.marked[s.Key()] {
				delete(m.marked, s.Key())
			} else {
				m.marked[s.Key()] = true
			}
			m.flash(fmt.Sprintf("%d marked — press c to share their context, esc to clear", len(m.marked)), false)
			m.move(1)
		}
	case "u":
		m.marked = map[string]bool{}
	case "c":
		if m.busy != "" {
			m.flash("a handoff is already being written — wait for it to finish", true)
			break
		}
		m.share = m.shareSources()
		if len(m.share) == 0 {
			m.flash("select or mark a session with a transcript first", true)
			break
		}
		m.mode = modeShareMenu
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup", "ctrl+u":
		m.move(-m.listHeight() / 2)
	case "pgdown", "ctrl+d":
		m.move(m.listHeight() / 2)
	case "home":
		m.move(-len(m.rows))
	case "end":
		m.move(len(m.rows))
	case "tab", "L":
		m.setTab((m.tab + 1) % tab(m.numTabs()))
	case "shift+tab", "H":
		m.setTab((m.tab + tab(m.numTabs()) - 1) % tab(m.numTabs()))
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if t := tab(key[0] - '1'); int(t) < m.numTabs() {
			m.setTab(t)
		}
	case "+", "=":
		keys := m.groupTargets()
		if len(keys) == 0 {
			m.flash("move to a session (or mark some) first", true)
			break
		}
		hint := "new name"
		if len(m.store.Groups) > 0 {
			hint = "existing: " + strings.Join(m.store.Groups, ", ")
		}
		prefill := ""
		if g, ok := m.currentGroup(); ok {
			prefill = g
		}
		m.openInput(modeGroupAdd, fmt.Sprintf("Add %s to group (%s): ", plural(len(keys), "session"), hint), prefill)
		return m, textinput.Blink
	case "-":
		g, ok := m.currentGroup()
		if !ok {
			m.flash("open a group's tab to remove sessions from it", true)
			break
		}
		keys := m.groupTargets()
		if len(keys) == 0 {
			m.flash("move to a session (or mark some) first", true)
			break
		}
		m.store.RemoveFromGroup(keys, g)
		m.marked = map[string]bool{}
		m.flash(fmt.Sprintf("removed %s from %s", plural(len(keys), "session"), g), false)
		m.rebuild()
	case "E":
		if g, ok := m.currentGroup(); ok {
			m.openInput(modeGroupRename, "Rename group: ", g)
			return m, textinput.Blink
		}
		m.flash("open a group's tab to rename it", true)
	case "X":
		if _, ok := m.currentGroup(); ok {
			m.mode = modeConfirmGroupDelete
		} else {
			m.flash("open a group's tab to delete it", true)
		}
	case "/":
		m.mode = modeSearch
		m.search.Focus()
		return m, textinput.Blink
	case "g":
		m.grouped = !m.grouped
		m.rebuild()
		if m.grouped {
			m.flash("grouped by project — enter/space on a project folds it", false)
		}
	case " ", "left", "right":
		if r := m.cur(); r != nil && r.header != "" {
			m.collapsed[r.header] = !m.collapsed[r.header]
			m.rebuild()
		} else if key == "left" && m.grouped {
			// fold the group the cursor is in
			if s := m.selected(); s != nil {
				m.collapsed[projectKey(s.Cwd)] = true
				m.rebuild()
				m.selectHeader(projectKey(s.Cwd))
			}
		}
	case "enter", "o":
		if r := m.cur(); r != nil && r.header != "" {
			m.collapsed[r.header] = !m.collapsed[r.header]
			m.rebuild()
			return m, nil
		}
		return m, m.openSelected()
	case "n":
		m.mode = modeNewAgent
	case "r":
		if s := m.selected(); s != nil {
			m.openInput(modeRename, "Rename: ", m.displayTitle(s))
			return m, textinput.Blink
		}
	case "t":
		if s := m.selected(); s != nil {
			m.openInput(modeTags, "Tags (comma/space separated): ", strings.Join(m.store.Get(s.Key()).Tags, ", "))
			return m, textinput.Blink
		}
	case "p":
		if s := m.selected(); s != nil {
			pinned := false
			m.store.Update(s.Key(), func(mt *state.Meta) { mt.Pinned = !mt.Pinned; pinned = mt.Pinned })
			m.flash(map[bool]string{true: "pinned", false: "unpinned"}[pinned], false)
			m.rebuild()
		}
	case "a", "d":
		if s := m.selected(); s != nil {
			arch := false
			m.store.Update(s.Key(), func(mt *state.Meta) { mt.Archived = !mt.Archived; arch = mt.Archived })
			if arch {
				m.flash("archived (hidden) — see the Archived tab to restore; agent files untouched", false)
			} else {
				m.flash("restored from archive", false)
			}
			m.rebuild()
		}
	case "x":
		if s := m.selected(); s != nil {
			if p := m.liveBy[s.Key()]; p != nil {
				m.mode = modeConfirmStop
			} else {
				m.flash("not running", true)
			}
		}
	case "y":
		if s := m.selected(); s != nil {
			dir, _ := term.ExistingDir(s.Cwd)
			c := "cd " + term.Quote(dir) + " && " + resumeCommand(s)
			if err := term.CopyToClipboard(c); err != nil {
				m.flash(err.Error(), true)
			} else {
				m.flash("copied: "+c, false)
			}
		}
	case "h":
		m.showSub = !m.showSub
		m.rebuild()
		m.flash(map[bool]string{true: "showing Codex subagent threads", false: "hiding Codex subagent threads"}[m.showSub], false)
	case "s":
		m.mode = modeStats
	case "?":
		m.mode = modeHelp
	case "R":
		if !m.loading {
			m.loading = true
			m.flash("rescanning sessions…", false)
			return m, tea.Batch(m.loadCmd(), liveCmd())
		}
	case "[", "shift+up":
		m.pScroll += 5
	case "]", "shift+down":
		m.pScroll = max(0, m.pScroll-5)
	case "{":
		m.pScroll += 25
	case "}":
		m.pScroll = max(0, m.pScroll-25)
	}
	if m.cursor != prev {
		m.pScroll = 0
	}
	return m, m.ensurePreview(false)
}

const listTop = 3 // header, tabs, pane border

func (m Model) handleMouse(e tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeNormal || e.Action != tea.MouseActionPress {
		return m, nil
	}
	overList := e.X < m.listWidth()
	prev := m.cursor
	switch e.Button {
	case tea.MouseButtonWheelUp:
		if overList {
			m.move(-3)
		} else {
			m.pScroll += 3
		}
	case tea.MouseButtonWheelDown:
		if overList {
			m.move(3)
		} else {
			m.pScroll = max(0, m.pScroll-3)
		}
	case tea.MouseButtonLeft:
		i := m.offset + e.Y - listTop
		if !overList || e.Y < listTop || e.Y >= listTop+m.listHeight() || i < 0 || i >= len(m.rows) {
			return m, nil
		}
		if i == m.cursor && m.rows[i].header != "" {
			m.collapsed[m.rows[i].header] = !m.collapsed[m.rows[i].header]
			m.rebuild()
		}
		m.cursor = i
		m.clampScroll()
	default:
		return m, nil
	}
	if m.cursor != prev {
		m.pScroll = 0
	}
	return m, m.ensurePreview(false)
}

func (m *Model) openInput(md mode, prompt, value string) {
	m.mode = md
	m.input.Prompt = prompt
	m.input.SetValue(value)
	m.input.CursorEnd()
	m.input.Focus()
}

func (m Model) submitInput() (tea.Model, tea.Cmd) {
	val := strings.TrimSpace(m.input.Value())
	md := m.mode
	m.mode = modeNormal
	m.input.Blur()
	switch md {
	case modeRename:
		if s := m.selected(); s != nil {
			name := val
			if name == oneLine(s.DisplayTitle()) {
				name = ""
			}
			m.store.Update(s.Key(), func(mt *state.Meta) { mt.Name = name })
			m.flash("renamed (shown in agentdeck only)", false)
			m.rebuild()
		}
	case modeTags:
		if s := m.selected(); s != nil {
			tags := state.ParseTags(val)
			m.store.Update(s.Key(), func(mt *state.Meta) { mt.Tags = tags })
			m.flash("tags saved", false)
			m.rebuild()
		}
	case modeGroupAdd:
		if val == "" {
			return m, nil
		}
		keys := m.groupTargets()
		name, err := m.store.AddToGroup(keys, val)
		if err != nil {
			m.flash(err.Error(), true)
			return m, nil
		}
		m.marked = map[string]bool{}
		m.flash(fmt.Sprintf("added %s to %s — it's tab %d", plural(len(keys), "session"), name, m.groupTab(name)+1), false)
		m.rebuild()
	case modeGroupRename:
		if g, ok := m.currentGroup(); ok {
			if err := m.store.RenameGroup(g, val); err != nil {
				m.flash(err.Error(), true)
			} else {
				m.flash("group renamed", false)
			}
			m.rebuild()
		}
	case modeShareDir:
		dir := expandHome(val)
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			m.flash("not a folder: "+val, true)
			m.share = nil
			return m, nil
		}
		return m, m.startHandoff(shareNew, m.shareAg, dir, nil)
	case modeNewDir:
		dir := expandHome(val)
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			m.flash("not a folder: "+val, true)
			return m, nil
		}
		a := m.newAgent
		return m, func() tea.Msg {
			err := term.OpenTab(dir, config.Get().Command(string(a), string(a)), agentName(a)+" · "+filepath.Base(dir))
			return actionMsg{text: "opened new " + agentName(a) + " in " + session.ShortPath(dir), err: err, reload: true}
		}
	}
	return m, nil
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		h, _ := os.UserHomeDir()
		return filepath.Join(h, strings.TrimPrefix(p, "~"))
	}
	return p
}

// numTabs counts built-in tabs plus one per user group.
func (m *Model) numTabs() int { return int(tabFirstGroup) + len(m.store.Groups) }

// currentGroup returns the user group shown by the current tab, if any.
func (m *Model) currentGroup() (string, bool) {
	i := int(m.tab - tabFirstGroup)
	if i < 0 || i >= len(m.store.Groups) {
		return "", false
	}
	return m.store.Groups[i], true
}

func (m *Model) groupTab(name string) int {
	for i, g := range m.store.Groups {
		if g == name {
			return int(tabFirstGroup) + i
		}
	}
	return 0
}

// groupTargets is the marked sessions, or the selected one.
func (m *Model) groupTargets() []string {
	var keys []string
	for k := range m.marked {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		if s := m.selected(); s != nil {
			keys = append(keys, s.Key())
		}
	}
	sort.Strings(keys)
	return keys
}

func (m *Model) setTab(t tab) {
	if int(t) >= m.numTabs() {
		t = tabAll
	}
	m.tab = t
	m.cursor, m.offset, m.pScroll = 0, 0, 0
	m.rows = nil // don't carry the old selection into the new tab
	m.rebuild()
}

func (m *Model) move(d int) {
	if len(m.rows) == 0 {
		return
	}
	m.cursor = min(max(m.cursor+d, 0), len(m.rows)-1)
	m.clampScroll()
}

func (m *Model) clampScroll() {
	h := m.listHeight()
	if h <= 0 {
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	m.offset = max(0, min(m.offset, max(0, len(m.rows)-h)))
}

func (m *Model) cur() *row {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return &m.rows[m.cursor]
}

func (m *Model) selected() *session.Session {
	if r := m.cur(); r != nil {
		return r.s
	}
	return nil
}

func (m *Model) selectHeader(h string) {
	for i, r := range m.rows {
		if r.header == h {
			m.cursor = i
			m.clampScroll()
			return
		}
	}
}

// ensurePreview loads the selected session's conversation when missing or stale.
func (m *Model) ensurePreview(force bool) tea.Cmd {
	s := m.selected()
	if s == nil || s.Path == "" {
		return nil
	}
	key := s.Key()
	mt := m.mtimes[key]
	if mt.IsZero() {
		mt = s.Updated
	}
	if m.inflight[key] {
		return nil
	}
	if p := m.preview[key]; p != nil && !force && !mt.After(p.mtime) {
		return nil
	}
	m.inflight[key] = true
	return previewCmd(key, s)
}

// ---- live mapping ----------------------------------------------------------

func projectKey(cwd string) string {
	if i := strings.Index(cwd, "/.claude/worktrees/"); i >= 0 {
		return cwd[:i]
	}
	if i := strings.Index(cwd, "/.worktrees/"); i >= 0 {
		return cwd[:i]
	}
	return cwd
}

func (m *Model) mapLive() {
	m.liveBy = map[string]*live.Proc{}
	m.orphans = nil
	byKey := map[string]*session.Session{}
	byPath := map[string]*session.Session{}
	for _, s := range m.sessions {
		byKey[s.Key()] = s
		byPath[s.Path] = s
	}
	used := map[string]bool{}
	for i := range m.procs {
		p := &m.procs[i]
		var s *session.Session
		switch p.Agent {
		case "claude":
			s = byKey["claude:"+p.SessionID]
			if s == nil {
				title := p.Name
				if title == "" || title == p.JobID {
					title = "new Claude session"
				}
				s = &session.Session{Agent: session.Claude, ID: p.SessionID, Cwd: p.Cwd, Title: title, Created: p.Started, Updated: p.Started}
				m.orphans = append(m.orphans, s)
			}
		default:
			ag := session.Agent(p.Agent)
			if p.File != "" {
				s = byPath[p.File]
			}
			if s == nil && p.Cwd != "" {
				// Best guess: the most recently active session of this agent in the same folder.
				for _, c := range m.sessions {
					if c.Agent == ag && c.Cwd == p.Cwd && !used[c.Key()] && c.Updated.After(p.Started.Add(-2*time.Minute)) {
						s = c
						break
					}
				}
			}
			if s == nil {
				s = &session.Session{Agent: ag, ID: fmt.Sprintf("pid-%d", p.PID), Cwd: p.Cwd,
					Title: fmt.Sprintf("new %s session (pid %d)", agentName(ag), p.PID), Created: p.Started, Updated: p.Started}
				m.orphans = append(m.orphans, s)
			}
			if s.Path != "" {
				mt := session.ModTime(s.Path)
				m.mtimes[s.Key()] = mt
				if time.Since(mt) < workingWindow {
					p.Status = live.Working
				} else {
					p.Status = live.Idle
				}
			}
		}
		used[s.Key()] = true
		m.liveBy[s.Key()] = p
		if p.Agent == "claude" && s.Path != "" {
			m.mtimes[s.Key()] = session.ModTime(s.Path)
		}
	}
}

// ---- filtering / rows --------------------------------------------------------

func (m *Model) displayTitle(s *session.Session) string {
	if n := m.store.Get(s.Key()).Name; n != "" {
		return oneLine(n)
	}
	return oneLine(s.DisplayTitle())
}

func (m *Model) activity(s *session.Session) time.Time {
	if mt := m.mtimes[s.Key()]; mt.After(s.Updated) {
		return mt
	}
	return s.Updated
}

func (m *Model) visible(s *session.Session, terms []string) bool {
	meta := m.store.Get(s.Key())
	p := m.liveBy[s.Key()]
	if m.tab == tabArchived {
		if !meta.Archived {
			return false
		}
	} else if meta.Archived {
		return false
	}
	if s.Subagent && !m.showSub && m.query == "" {
		return false
	}
	switch m.tab {
	case tabLive:
		if p == nil {
			return false
		}
	case tabClaude:
		if s.Agent != session.Claude {
			return false
		}
	case tabCodex:
		if s.Agent != session.Codex {
			return false
		}
	case tabPi:
		if s.Agent != session.Pi {
			return false
		}
	case tabPinned:
		if !meta.Pinned {
			return false
		}
	}
	if g, ok := m.currentGroup(); ok && !meta.InGroup(g) {
		return false
	}
	if len(terms) == 0 {
		return true
	}
	hay := strings.ToLower(strings.Join([]string{m.displayTitle(s), s.Title, s.FirstPrompt, s.LastPrompt,
		s.Cwd, s.Model, s.Branch, string(s.Agent), s.ID, s.Source}, "\n"))
	for _, t := range terms {
		if strings.HasPrefix(t, "@") && len(t) > 1 {
			found := false
			for _, g := range meta.Groups {
				if strings.HasPrefix(strings.ToLower(g), t[1:]) {
					found = true
				}
			}
			if !found {
				return false
			}
		} else if strings.HasPrefix(t, "#") && len(t) > 1 {
			found := false
			for _, tg := range meta.Tags {
				if strings.HasPrefix(tg, t[1:]) {
					found = true
				}
			}
			if !found {
				return false
			}
		} else if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
}

func (m *Model) rebuild() {
	var keep string
	if r := m.cur(); r != nil {
		if r.s != nil {
			keep = r.s.Key()
		} else {
			keep = "#" + r.header
		}
	}
	terms := strings.Fields(strings.ToLower(m.query))
	all := append(append([]*session.Session{}, m.orphans...), m.sessions...)
	var list []*session.Session
	for _, s := range all {
		if m.visible(s, terms) {
			list = append(list, s)
		}
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if !m.grouped && m.tab != tabPinned {
			pa, pb := m.store.Get(a.Key()).Pinned, m.store.Get(b.Key()).Pinned
			if pa != pb {
				return pa
			}
		}
		if m.tab == tabLive {
			wa, wb := m.liveBy[a.Key()].Status == live.Working, m.liveBy[b.Key()].Status == live.Working
			if wa != wb {
				return wa
			}
		}
		return m.activity(a).After(m.activity(b))
	})

	m.rows = m.rows[:0]
	if !m.grouped {
		pinned := 0
		if m.tab != tabPinned {
			for _, s := range list {
				if m.store.Get(s.Key()).Pinned {
					pinned++
				}
			}
		}
		for i, s := range list {
			if pinned > 0 && i == 0 {
				m.rows = append(m.rows, row{header: secPinned, section: true, count: pinned})
			}
			if pinned > 0 && i == pinned && i < len(list) {
				m.rows = append(m.rows, row{header: secRecent, section: true, count: len(list) - pinned})
			}
			if pinned > 0 && m.collapsed[map[bool]string{true: secPinned, false: secRecent}[i < pinned]] {
				continue
			}
			m.rows = append(m.rows, row{s: s})
		}
	} else {
		groups := map[string][]*session.Session{}
		var order []string
		for _, s := range list {
			k := projectKey(s.Cwd)
			if _, ok := groups[k]; !ok {
				order = append(order, k)
			}
			groups[k] = append(groups[k], s)
		}
		for _, k := range order {
			g := groups[k]
			m.rows = append(m.rows, row{header: k, count: len(g), latest: m.activity(g[0])})
			if !m.collapsed[k] {
				for _, s := range g {
					m.rows = append(m.rows, row{s: s})
				}
			}
		}
	}
	m.cursor = min(m.cursor, max(0, len(m.rows)-1))
	if keep == "" && m.cursor == 0 && len(m.rows) > 1 && m.rows[0].section {
		m.cursor = 1 // fresh list: land on the first session, not the ★ Pinned divider
	}
	if keep != "" {
		for i, r := range m.rows {
			if (r.s != nil && r.s.Key() == keep) || (r.s == nil && "#"+r.header == keep) {
				m.cursor = i
				break
			}
		}
	}
	m.clampScroll()
}

// ---- actions -----------------------------------------------------------------

// resumeCommand is the shell line that resumes s, including any env from ~/.agentdeck/config.json.
func resumeCommand(s *session.Session) string {
	return config.Get().Command(string(s.Agent), baseResumeCommand(s))
}

func baseResumeCommand(s *session.Session) string {
	switch s.Agent {
	case session.Claude:
		return "claude --resume " + s.ID
	case session.Codex:
		return "codex resume " + s.ID
	case session.Pi:
		return "pi --session " + term.Quote(s.Path)
	}
	return ""
}

func (m *Model) openSelected() tea.Cmd {
	s := m.selected()
	if s == nil {
		return nil
	}
	p := m.liveBy[s.Key()]
	title := agentName(s.Agent) + " · " + trunc(m.displayTitle(s), 40)
	if p != nil {
		p := *p
		return func() tea.Msg {
			tty := p.TTY
			if p.Agent == "claude" && p.Kind == "bg" {
				tty = p.AttachedTTY
			}
			if tty != "" {
				ok, err := term.Focus(tty)
				if ok {
					return actionMsg{text: "jumped to running session on " + tty}
				}
				if err != nil {
					return actionMsg{err: err}
				}
			}
			if p.Agent == "claude" && p.Kind == "bg" && p.JobID != "" {
				dir, _ := term.ExistingDir(p.Cwd)
				err := term.OpenTab(dir, config.Get().Command("claude", "claude attach "+p.JobID), title)
				return actionMsg{text: "attached to background Claude job " + p.JobID, err: err, reload: true}
			}
			return actionMsg{err: fmt.Errorf("running as pid %d on %s, but no iTerm2 tab owns that terminal", p.PID, orDash(p.TTY))}
		}
	}
	if s.Path == "" {
		return nil
	}
	dir, exact := term.ExistingDir(s.Cwd)
	c := resumeCommand(s)
	return func() tea.Msg {
		err := term.OpenTab(dir, c, title)
		msg := "resumed in new iTerm2 tab"
		if !exact {
			msg += " (original folder is gone — opened in " + session.ShortPath(dir) + ")"
		}
		return actionMsg{text: msg, err: err, reload: true}
	}
}

func orDash(s string) string {
	if s == "" {
		return "no tty"
	}
	return s
}

func (m *Model) stopSelected() tea.Cmd {
	s := m.selected()
	if s == nil {
		return nil
	}
	p := m.liveBy[s.Key()]
	if p == nil {
		return nil
	}
	p2 := *p
	return func() tea.Msg {
		if p2.Agent == "claude" && p2.Kind == "bg" && p2.JobID != "" {
			cmd := exec.Command("claude", "stop", p2.JobID)
			cmd.Env = config.Get().Environ("claude")
			out, err := cmd.CombinedOutput()
			if err != nil {
				return actionMsg{err: fmt.Errorf("claude stop %s: %s", p2.JobID, strings.TrimSpace(string(out))), reload: true}
			}
			return actionMsg{text: "stopped background Claude job " + p2.JobID, reload: true}
		}
		if err := syscall.Kill(p2.PID, syscall.SIGTERM); err != nil {
			return actionMsg{err: err, reload: true}
		}
		return actionMsg{text: fmt.Sprintf("sent SIGTERM to %s (pid %d)", p2.Agent, p2.PID), reload: true}
	}
}

// ---- context sharing -----------------------------------------------------------

// shareSources returns the marked sessions (oldest activity first), or just the selected one.
func (m *Model) shareSources() []*session.Session {
	var out []*session.Session
	if len(m.marked) > 0 {
		for _, s := range append(append([]*session.Session{}, m.orphans...), m.sessions...) {
			if m.marked[s.Key()] && s.Path != "" {
				out = append(out, s)
			}
		}
		sort.SliceStable(out, func(i, j int) bool { return m.activity(out[i]).Before(m.activity(out[j])) })
		return out
	}
	if s := m.selected(); s != nil && s.Path != "" {
		out = append(out, s)
	}
	return out
}

func (m *Model) startHandoff(action shareAction, agent session.Agent, dir string, target *live.Proc) tea.Cmd {
	sources := m.share
	m.share = nil
	if len(sources) == 0 {
		return nil
	}
	m.marked = map[string]bool{}
	m.busy = fmt.Sprintf("writing context handoff from %d session(s)", len(sources))
	m.busySince = time.Now()
	return func() tea.Msg {
		res, err := handoff.Generate(context.Background(), sources)
		return handoffMsg{res: res, err: err, action: action, agent: agent, dir: dir, target: target, sources: len(sources)}
	}
}

func deliverHandoff(h handoffMsg) tea.Cmd {
	return func() tea.Msg {
		note := session.ShortPath(h.res.Path)
		switch h.action {
		case shareCopy:
			if err := term.CopyToClipboard(h.res.Note); err != nil {
				return actionMsg{err: err}
			}
			return actionMsg{text: "handoff copied to clipboard · saved at " + note}
		case shareNew:
			title := "↪ " + agentName(h.agent) + " · " + filepath.Base(h.dir)
			err := term.OpenTab(h.dir, config.Get().Command(string(h.agent), handoff.StartCommand(h.agent, h.res.Path)), title)
			return actionMsg{text: "started " + agentName(h.agent) + " with the context in a new tab · " + note, err: err, reload: true}
		case shareSend:
			p := h.target
			text := handoff.SendText(h.res.Path)
			tty := p.TTY
			if p.Agent == "claude" && p.Kind == "bg" {
				tty = p.AttachedTTY
			}
			if tty != "" {
				ok, err := term.TypeInto(tty, text)
				if err != nil {
					return actionMsg{err: err}
				}
				if ok {
					return actionMsg{text: "typed into the session on " + tty + " — review it there and press enter"}
				}
			}
			// No iTerm tab owns this session: put the line on the clipboard and open/attach a tab for it.
			if err := term.CopyToClipboard(text); err != nil {
				return actionMsg{err: err}
			}
			if p.Agent == "claude" && p.Kind == "bg" && p.JobID != "" {
				dir, _ := term.ExistingDir(p.Cwd)
				err := term.OpenTab(dir, config.Get().Command("claude", "claude attach "+p.JobID), "Claude · "+p.JobID)
				return actionMsg{text: "attached the job in a new tab — paste (⌘V) and press enter to send the context", err: err, reload: true}
			}
			return actionMsg{text: "no iTerm2 tab found for it — the message is on your clipboard, paste it there"}
		}
		return nil
	}
}
