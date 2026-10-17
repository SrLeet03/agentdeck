// Package handoff turns one or more sessions into an AI-written context note another agent can pick up.
package handoff

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/SrLeet03/agentdeck/internal/config"
	"github.com/SrLeet03/agentdeck/internal/session"
)

const (
	// Budget for the transcript sent to the summarizer (~100K tokens).
	maxTranscript = 400_000
	maxAssistant  = 3_000
	maxUser       = 6_000
	maxTool       = 200
	timeout       = 4 * time.Minute
)

// Model used for summaries; any `claude --model` alias works.
var Model = "sonnet"

const instructions = `You are writing a context handoff note so a different coding agent (Claude Code, Codex or Pi) can continue this work without seeing the original conversation.
Below are transcripts of one or more coding-agent sessions (user prompts, agent replies, and one-line tool calls; older parts may be cut).

Write concise markdown with exactly these sections:
## Goal
## Current state
## Key decisions & constraints
## Important files, commands & IDs
## Open problems / next steps

Be concrete: absolute paths, branch names, commands, ticket/PR numbers, env names, error messages. Note anything the user explicitly asked NOT to do.
If several sessions are included, explain how they relate. Do not invent facts. Stay under ~700 words. Output only the note.`

// Result is a generated handoff.
type Result struct {
	Path   string // markdown file in ~/.agentdeck/handoffs
	Note   string // full file contents (header + summary)
	Cached bool
}

// Dir is where handoff notes are stored.
func Dir() string {
	d := filepath.Join(session.DataDir(), "handoffs")
	os.MkdirAll(d, 0o755)
	return d
}

// Transcript renders sessions as plain text for the summarizer, keeping the most recent part when too long.
func Transcript(sources []*session.Session) string {
	per := maxTranscript / max(1, len(sources))
	var b strings.Builder
	for _, s := range sources {
		msgs, truncated := session.Messages(s)
		var body strings.Builder
		for _, m := range msgs {
			text := strings.TrimSpace(m.Text)
			switch m.Role {
			case "user":
				fmt.Fprintf(&body, "\nUSER: %s\n", cut(text, maxUser))
			case "assistant":
				fmt.Fprintf(&body, "\nAGENT: %s\n", cut(text, maxAssistant))
			case "tool":
				fmt.Fprintf(&body, "  [tool] %s\n", cut(strings.Join(strings.Fields(text), " "), maxTool))
			}
		}
		t := body.String()
		if len(t) > per {
			t = "[… earlier conversation omitted …]\n" + t[len(t)-per:]
			truncated = true
		} else if truncated {
			t = "[… earlier conversation omitted …]\n" + t
		}
		fmt.Fprintf(&b, "\n===== SESSION: %s =====\nagent: %s · folder: %s · branch: %s · model: %s\nstarted: %s · last active: %s\n%s\n",
			oneLine(s.DisplayTitle()), s.Agent, s.Cwd, orNone(s.Branch), orNone(s.Model),
			s.Created.Local().Format(time.RFC1123), s.Updated.Local().Format(time.RFC1123), t)
	}
	return b.String()
}

// Generate summarizes sources with `claude -p`, reusing a previous note when no source changed since.
func Generate(ctx context.Context, sources []*session.Session) (Result, error) {
	if len(sources) == 0 {
		return Result{}, fmt.Errorf("no sessions selected")
	}
	path := filepath.Join(Dir(), cacheName(sources))
	if b, err := os.ReadFile(path); err == nil {
		return Result{Path: path, Note: string(b), Cached: true}, nil
	}
	if _, err := exec.LookPath("claude"); err != nil {
		return Result{}, fmt.Errorf("claude CLI not found on PATH (needed to write the summary)")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "claude", "-p", "--no-session-persistence", "--tools", "", "--model", Model, instructions)
	cmd.Dir = session.DataDir() // keep claude's per-folder bookkeeping out of real projects
	cmd.Env = config.Get().Environ("claude")
	cmd.Stdin = strings.NewReader(Transcript(sources))
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return Result{}, fmt.Errorf("summary timed out after %s", timeout)
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(string(out))
		}
		return Result{}, fmt.Errorf("claude -p failed: %s", cut(oneLine(msg), 200))
	}
	summary := strings.TrimSpace(string(out))
	if summary == "" {
		return Result{}, fmt.Errorf("claude -p returned an empty summary")
	}
	note := header(sources) + summary + "\n"
	if err := os.WriteFile(path, []byte(note), 0o644); err != nil {
		return Result{}, err
	}
	return Result{Path: path, Note: note}, nil
}

func header(sources []*session.Session) string {
	var b strings.Builder
	b.WriteString("# Context handoff (from agentdeck)\n\n")
	b.WriteString("This is context carried over from earlier coding-agent session(s). Read it, reply with a short summary of what you understood, then wait for my instructions — do not start changing anything yet.\n\n")
	b.WriteString("Sources:\n")
	for _, s := range sources {
		fmt.Fprintf(&b, "- %s session \"%s\" in %s (id %s, last active %s)\n", s.Agent, cut(oneLine(s.DisplayTitle()), 80),
			s.Cwd, s.ID, s.Updated.Local().Format("Jan 2 15:04"))
	}
	b.WriteString("\nFull transcripts (JSONL, one event per line; open them if you need exact details, commands or code):\n")
	for _, s := range sources {
		fmt.Fprintf(&b, "- %s\n", s.Path)
	}
	b.WriteString("\n")
	return b.String()
}

// cacheName is stable for the same sources at the same modification times.
func cacheName(sources []*session.Session) string {
	h := sha1.New()
	for _, s := range sources {
		fmt.Fprintf(h, "v2|%s|%d|", s.Key(), session.ModTime(s.Path).UnixNano()) // v2: notes link transcripts
	}
	slug := slugRe.ReplaceAllString(strings.ToLower(oneLine(sources[0].DisplayTitle())), "-")
	slug = strings.Trim(cut(slug, 40), "-…")
	if slug == "" {
		slug = "session"
	}
	return fmt.Sprintf("%s-%s-%s.md", time.Now().Format("20060102"), slug, hex.EncodeToString(h.Sum(nil))[:10])
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func orNone(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// StartCommand launches agent with the note as its first message.
func StartCommand(agent session.Agent, notePath string) string {
	return string(agent) + ` "$(cat ` + shellQuote(notePath) + `)"`
}

// SendText is what gets typed into an already-running agent (single line, so it doesn't submit early).
func SendText(notePath string) string {
	return "Read the context handoff at " + notePath + " and use it as background for what we do next. Briefly confirm what you understood."
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
