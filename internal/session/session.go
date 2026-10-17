// Package session discovers and parses Claude Code, Codex and Pi session logs.
package session

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Agent string

const (
	Claude Agent = "claude"
	Codex  Agent = "codex"
	Pi     Agent = "pi"
)

func (a Agent) Short() string {
	switch a {
	case Claude:
		return "CC"
	case Codex:
		return "CX"
	case Pi:
		return "PI"
	}
	return "??"
}

// Tokens are normalised so that In never includes cached input.
type Tokens struct {
	In         int64 `json:"in"`
	Out        int64 `json:"out"`
	CacheRead  int64 `json:"cr"`
	CacheWrite int64 `json:"cw"`
}

func (t Tokens) Total() int64 { return t.In + t.Out + t.CacheRead + t.CacheWrite }

func (t *Tokens) Add(o Tokens) {
	t.In += o.In
	t.Out += o.Out
	t.CacheRead += o.CacheRead
	t.CacheWrite += o.CacheWrite
}

type Session struct {
	Agent       Agent            `json:"agent"`
	ID          string           `json:"id"`
	Path        string           `json:"path"`
	Cwd         string           `json:"cwd"`
	Title       string           `json:"title"`
	FirstPrompt string           `json:"first_prompt"`
	LastPrompt  string           `json:"last_prompt"`
	Model       string           `json:"model"`
	Branch      string           `json:"branch"`
	Source      string           `json:"source"` // e.g. codex-tui, codex_work_desktop, sdk-cli
	Created     time.Time        `json:"created"`
	Updated     time.Time        `json:"updated"`
	UserMsgs    int              `json:"user_msgs"`
	AgentMsgs   int              `json:"agent_msgs"`
	Tokens      Tokens           `json:"tokens"`
	Daily       map[string]int64 `json:"daily"` // YYYY-MM-DD (local) -> tokens
	Subagent    bool             `json:"subagent"`
	Subagents   int              `json:"subagents"`
	ParentID    string           `json:"parent_id"`
}

// Key uniquely identifies a session across agents.
func (s *Session) Key() string { return string(s.Agent) + ":" + s.ID }

// DisplayTitle picks the best human label available.
func (s *Session) DisplayTitle() string {
	for _, t := range []string{s.Title, s.FirstPrompt} {
		if t = oneLine(t); t != "" {
			return t
		}
	}
	return "(untitled " + s.ID[:min(8, len(s.ID))] + ")"
}

// Project returns a short project label for the session's cwd.
func (s *Session) Project() string { return ShortPath(s.Cwd) }

func ShortPath(p string) string {
	if p == "" {
		return "?"
	}
	home := homeDir()
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+"/") {
		return "~/" + strings.TrimPrefix(p, home+"/")
	}
	return p
}

func (s *Session) ProjectBase() string {
	if s.Cwd == "" {
		return "?"
	}
	if s.Cwd == homeDir() {
		return "~"
	}
	return filepath.Base(s.Cwd)
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func addDaily(m map[string]int64, ts time.Time, n int64) {
	if ts.IsZero() || n == 0 {
		return
	}
	m[ts.Local().Format("2006-01-02")] += n
}

var tagBlock = regexp.MustCompile(`^<[a-zA-Z][\w-]*(\s[^>]*)?>`)

// isInjected reports whether a "user" message is harness-injected context rather than something typed.
func isInjected(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" || tagBlock.MatchString(t) {
		return true
	}
	for _, p := range []string{"<environment_context", "<user_instructions", "# AGENTS.md", "<command-", "<local-command", "<system-reminder", "<permissions", "<turn_aborted", "<user_shell_command", "Caveat: The messages below", "[Request interrupted", "<task-notification", "<bash-", "<collaboration_mode", "<subagent", "<skill"} {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

// commandName extracts "/foo args" from Claude's <command-name> wrapper so slash commands still read sensibly.
func commandName(text string) string {
	i := strings.Index(text, "<command-name>")
	if i < 0 {
		return ""
	}
	rest := text[i+len("<command-name>"):]
	j := strings.Index(rest, "</command-name>")
	if j < 0 {
		return ""
	}
	cmd := rest[:j]
	if a := strings.Index(text, "<command-args>"); a >= 0 {
		r := text[a+len("<command-args>"):]
		if b := strings.Index(r, "</command-args>"); b > 0 {
			cmd += " " + r[:b]
		}
	}
	return strings.TrimSpace(cmd)
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
