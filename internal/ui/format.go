package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/SrLeet03/agentdeck/internal/live"
	"github.com/SrLeet03/agentdeck/internal/session"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func agentColor(a session.Agent) lipgloss.Color {
	switch a {
	case session.Claude:
		return colClaude
	case session.Codex:
		return colCodex
	}
	return colPi
}

func agentBadge(a session.Agent) string {
	return lipgloss.NewStyle().Bold(true).Foreground(agentColor(a)).Render(a.Short())
}

func agentName(a session.Agent) string {
	switch a {
	case session.Claude:
		return "Claude Code"
	case session.Codex:
		return "Codex"
	}
	return "Pi"
}

func statusGlyph(p *live.Proc) string {
	if p == nil {
		return " "
	}
	switch p.Status {
	case live.Working:
		return stGreen.Render("●")
	case live.Idle:
		return stYellow.Render("◐")
	case live.Shell:
		return stAccent.Render("$")
	}
	return stDim.Render("○")
}

func statusLabel(p *live.Proc) string {
	switch p.Status {
	case live.Working:
		return stGreen.Render("● working")
	case live.Idle:
		return stYellow.Render("◐ idle — waiting for you")
	case live.Shell:
		return stAccent.Render("$ shell mode")
	}
	return stDim.Render("○ running")
}

func ago(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
	return t.Local().Format("Jan 2")
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("Mon Jan 2 15:04")
}

func humanTokens(n int64) string {
	f := float64(n)
	switch {
	case n >= 1e9:
		return fmt.Sprintf("%.1fB", f/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%.1fM", f/1e6)
	case n >= 1e3:
		return fmt.Sprintf("%.1fK", f/1e3)
	}
	return fmt.Sprintf("%d", n)
}

// trunc cuts s (which may contain ANSI codes) to w display cells.
func trunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}

// pad truncates or right-pads s to exactly w cells.
func pad(s string, w int) string {
	s = trunc(s, w)
	if n := lipgloss.Width(s); n < w {
		s += strings.Repeat(" ", w-n)
	}
	return s
}

func oneLine(s string) string { return strings.Join(strings.Fields(clean(s)), " ") }

// clean makes text from session logs safe to draw: it strips ANSI escape sequences, turns carriage
// returns into newlines and drops other control characters, any of which would move the terminal
// cursor and corrupt the screen.
func clean(s string) string {
	s = ansi.Strip(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return r
		case r == '\r':
			return '\n'
		case r == '\t':
			return ' '
		case r < 0x20, r == 0x7f, r >= 0x80 && r < 0xa0, r == 0x200b, r == 0xfeff:
			return -1
		}
		return r
	}, s)
}
