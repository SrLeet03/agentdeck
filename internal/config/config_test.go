package config

import (
	"strings"
	"testing"
)

func foundryOff() Config {
	return Config{Env: map[string]map[string]string{"claude": {"CLAUDE_CODE_USE_FOUNDRY": "0"}}}
}

func TestDefaultLeavesCommandsAlone(t *testing.T) {
	if got := Default().Command("claude", "claude"); got != "claude" {
		t.Fatalf("got %q", got)
	}
}

func TestCommandPrefixesExports(t *testing.T) {
	c := foundryOff()
	got := c.Command("claude", "claude --resume abc")
	if got != "export CLAUDE_CODE_USE_FOUNDRY=0 && claude --resume abc" {
		t.Fatalf("got %q", got)
	}
	if got := c.Command("codex", "codex resume x"); got != "codex resume x" {
		t.Fatalf("codex should be untouched, got %q", got)
	}
	c.Env["pi"] = map[string]string{"B": "two words", "A": "1"}
	if got := c.Command("pi", "pi"); got != "export A=1 && export B='two words' && pi" {
		t.Fatalf("got %q", got)
	}
}

func TestEnvironOverridesInheritedValue(t *testing.T) {
	t.Setenv("CLAUDE_CODE_USE_FOUNDRY", "1")
	env := foundryOff().Environ("claude")
	n := 0
	for _, kv := range env {
		if strings.HasPrefix(kv, "CLAUDE_CODE_USE_FOUNDRY=") {
			n++
			if kv != "CLAUDE_CODE_USE_FOUNDRY=0" {
				t.Fatalf("got %s", kv)
			}
		}
	}
	if n != 1 {
		t.Fatalf("expected exactly one CLAUDE_CODE_USE_FOUNDRY, got %d", n)
	}
}
