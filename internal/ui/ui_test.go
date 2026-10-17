package ui

import (
	"strings"
	"testing"

	"github.com/SrLeet03/agentdeck/internal/session"
)

func TestProjectKeyFoldsWorktrees(t *testing.T) {
	cases := map[string]string{
		"/u/work/bob/.claude/worktrees/fix-x": "/u/work/bob",
		"/u/work/bob":                         "/u/work/bob",
		"/u/exp/app/.worktrees/feat":          "/u/exp/app",
	}
	for in, want := range cases {
		if got := projectKey(in); got != want {
			t.Errorf("projectKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanTokens(t *testing.T) {
	cases := map[int64]string{999: "999", 1500: "1.5K", 2_300_000: "2.3M", 4_500_000_000: "4.5B"}
	for in, want := range cases {
		if got := humanTokens(in); got != want {
			t.Errorf("humanTokens(%d) = %q, want %q", in, got, want)
		}
	}
}

// Rendering must never panic, even when the window is too short for the preview's details block.
func TestViewSurvivesSmallWindows(t *testing.T) {
	m := New()
	long := strings.Repeat("a very long session title that wraps ", 20)
	m.sessions = []*session.Session{{Agent: session.Claude, ID: "x", Path: "/nonexistent.jsonl", Cwd: "/tmp", Title: long, Branch: "main", Model: "m"}}
	m.loaded = true
	m.preview["claude:x"] = &previewData{msgs: []session.Message{{Role: "user", Text: long}}}
	m.rebuild()
	for _, size := range [][2]int{{60, 12}, {80, 14}, {200, 12}, {120, 40}} {
		m.w, m.h = size[0], size[1]
		for _, scroll := range []int{0, 5, 1000} {
			m.pScroll = scroll
			_ = m.View()
		}
	}
}

// Session text with carriage returns or escape codes must not reach the terminal, or it scrambles the screen.
func TestViewStripsControlCharacters(t *testing.T) {
	m := New()
	dirty := "Comment on lines +48\r\rChecks:\r- \x1b[31mred\x1b[0m done\x07\bok\tend"
	m.sessions = []*session.Session{{Agent: session.Claude, ID: "x", Path: "/nonexistent.jsonl", Cwd: "/tmp", Title: "t\rx", Branch: "b\x1b[2J"}}
	m.loaded = true
	m.preview["claude:x"] = &previewData{msgs: []session.Message{{Role: "user", Text: dirty}, {Role: "tool", Text: dirty}}}
	m.rebuild()
	m.w, m.h = 140, 30
	out := m.View()
	for _, bad := range []string{"\r", "\x07", "\b", "\x1b[2J", "\x1b[31m"} {
		if strings.Contains(out, bad) {
			t.Errorf("frame contains %q", bad)
		}
	}
	if n := strings.Count(out, "\n") + 1; n > m.h {
		t.Errorf("frame has %d lines, window has %d", n, m.h)
	}
	if got := clean(dirty); got != "Comment on lines +48\n\nChecks:\n- red doneok end" {
		t.Errorf("clean = %q", got)
	}
}
