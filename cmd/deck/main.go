// agentdeck (`deck`) is a terminal dashboard for Claude Code, Codex and Pi sessions.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/SrLeet03/agentdeck/internal/config"
	"github.com/SrLeet03/agentdeck/internal/handoff"
	"github.com/SrLeet03/agentdeck/internal/live"
	"github.com/SrLeet03/agentdeck/internal/session"
	"github.com/SrLeet03/agentdeck/internal/term"
	"github.com/SrLeet03/agentdeck/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

// version is set at release build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `deck — dashboard for your Claude Code, Codex and Pi sessions

usage:
  deck            open the dashboard
  deck list [n]   print the n most recent sessions (default 20)
  deck live       print running agent processes
  deck handoff <id-prefix>...   write an AI context note from session(s) and print its path
  deck doctor     check data folders and iTerm2 access
  deck version    print the version
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		p := tea.NewProgram(ui.New(), tea.WithAltScreen(), tea.WithMouseCellMotion())
		if _, err := p.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	switch args[0] {
	case "list", "ls":
		n := 20
		if len(args) > 1 {
			fmt.Sscan(args[1], &n)
		}
		list(n)
	case "live", "ps":
		printLive()
	case "doctor":
		doctor()
	case "handoff":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: deck handoff <session-id-prefix>...")
			os.Exit(2)
		}
		makeHandoff(args[1:])
	case "-v", "--version", "version":
		fmt.Println("deck", version)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

func list(n int) {
	l := session.NewLoader()
	ss := l.Load()
	l.Save()
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT\tACTIVE\tPROJECT\tTITLE\tID")
	for i, s := range ss {
		if i >= n {
			break
		}
		title := strings.Join(strings.Fields(s.DisplayTitle()), " ")
		if len([]rune(title)) > 60 {
			title = string([]rune(title)[:59]) + "…"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", s.Agent.Short(), s.Updated.Local().Format("Jan 02 15:04"), s.Project(), title, s.ID)
	}
	tw.Flush()
}

func printLive() {
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT\tPID\tSTATUS\tKIND\tTTY\tCWD\tSESSION")
	for _, p := range live.Scan() {
		id := p.SessionID
		if id == "" {
			id = p.File
		}
		kind := p.Kind
		if kind == "" {
			kind = "interactive"
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\t%s\n", p.Agent, p.PID, p.Status, kind, p.TTY, session.ShortPath(p.Cwd), id)
	}
	tw.Flush()
}

func doctor() {
	home, _ := os.UserHomeDir()
	for _, d := range []string{home + "/.claude/projects", home + "/.claude/sessions", home + "/.codex/sessions", home + "/.pi/agent/sessions"} {
		if _, err := os.Stat(d); err != nil {
			fmt.Println("✗", d, "missing")
		} else {
			fmt.Println("✓", d)
		}
	}
	t := time.Now()
	l := session.NewLoader()
	ss := l.Load()
	l.Save()
	c := map[session.Agent]int{}
	for _, s := range ss {
		c[s.Agent]++
	}
	fmt.Printf("✓ parsed %d sessions in %s (claude %d, codex %d, pi %d)\n", len(ss), time.Since(t).Round(time.Millisecond), c[session.Claude], c[session.Codex], c[session.Pi])
	fmt.Printf("✓ %d running agent processes\n", len(live.Scan()))
	fmt.Println("  state:", session.DataDir())
	fmt.Println("  config:", config.Path())
	for _, a := range []string{"claude", "codex", "pi"} {
		if env := config.Get().Env[a]; len(env) > 0 {
			fmt.Printf("  %s launches with %v\n", a, env)
		}
	}
	if err := term.Check(); err != nil {
		fmt.Println("✗ iTerm2 automation:", err)
		fmt.Println("  allow it in System Settings → Privacy & Security → Automation")
	} else {
		fmt.Println("✓ iTerm2 automation works")
	}
}

func makeHandoff(prefixes []string) {
	l := session.NewLoader()
	ss := l.Load()
	l.Save()
	var picked []*session.Session
	for _, p := range prefixes {
		var match []*session.Session
		for _, s := range ss {
			if strings.HasPrefix(s.ID, p) {
				match = append(match, s)
			}
		}
		if len(match) != 1 {
			fmt.Fprintf(os.Stderr, "%q matches %d sessions\n", p, len(match))
			os.Exit(1)
		}
		picked = append(picked, match[0])
	}
	t := time.Now()
	res, err := handoff.Generate(context.Background(), picked)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote handoff in %s (cached=%v)\n", time.Since(t).Round(time.Second), res.Cached)
	fmt.Println(res.Path)
}
