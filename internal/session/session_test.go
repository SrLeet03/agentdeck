package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsInjected(t *testing.T) {
	injected := []string{
		"<environment_context>\n<cwd>/x</cwd></environment_context>",
		"<in-app-browser-context source=\"ambient\">…",
		"# AGENTS.md instructions for /x",
		"<system-reminder>hi</system-reminder>",
		"   ",
	}
	for _, s := range injected {
		if !isInjected(s) {
			t.Errorf("expected injected: %q", s)
		}
	}
	typed := []string{"fix the build", "< 5 items please", "1. Think before coding", "/model"}
	for _, s := range typed {
		if isInjected(s) {
			t.Errorf("expected typed: %q", s)
		}
	}
}

func TestCommandName(t *testing.T) {
	in := "<command-name>/review</command-name>\n<command-message>review</command-message>\n<command-args>123</command-args>"
	if got := commandName(in); got != "/review 123" {
		t.Fatalf("got %q", got)
	}
}

func TestParseClaudeDedupesStreamedUsage(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "abc.jsonl")
	data := `{"type":"user","sessionId":"abc","cwd":"/w","timestamp":"2026-10-15T10:00:00Z","message":{"role":"user","content":"hello there"}}
{"type":"assistant","timestamp":"2026-10-15T10:00:01Z","message":{"id":"m1","role":"assistant","model":"claude-opus-5-5","content":[{"type":"thinking","thinking":""}],"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":100,"cache_creation_input_tokens":20}}}
{"type":"assistant","timestamp":"2026-10-15T10:00:02Z","message":{"id":"m1","role":"assistant","model":"claude-opus-5-5","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":100,"cache_creation_input_tokens":20}}}
{"type":"user","timestamp":"2026-10-15T10:00:03Z","message":{"role":"user","content":[{"type":"tool_result","content":"x"}]}}
{"type":"ai-title","aiTitle":"greeting test","sessionId":"abc"}
`
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := parseClaude(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "greeting test" || s.Cwd != "/w" || s.UserMsgs != 1 || s.AgentMsgs != 1 {
		t.Fatalf("unexpected session: %+v", s)
	}
	if s.Tokens != (Tokens{In: 10, Out: 5, CacheRead: 100, CacheWrite: 20}) {
		t.Fatalf("tokens not deduped: %+v", s.Tokens)
	}
}

func TestCodexUsageExcludesCachedFromInput(t *testing.T) {
	got := codexUsage{In: 1000, Cached: 800, Out: 50}.tokens()
	if got != (Tokens{In: 200, Out: 50, CacheRead: 800}) {
		t.Fatalf("got %+v", got)
	}
}
