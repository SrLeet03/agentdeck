package handoff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SrLeet03/agentdeck/internal/session"
)

func TestTranscriptKeepsTailAndLabelsRoles(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s1.jsonl")
	var b strings.Builder
	b.WriteString(`{"type":"session","id":"s1","timestamp":"2026-10-15T10:00:00Z","cwd":"/w"}` + "\n")
	b.WriteString(`{"type":"message","timestamp":"2026-10-15T10:00:01Z","message":{"role":"user","content":[{"type":"text","text":"FIRST-PROMPT"}]}}` + "\n")
	for i := 0; i < 400; i++ {
		b.WriteString(`{"type":"message","timestamp":"2026-10-15T10:00:02Z","message":{"role":"assistant","content":[{"type":"text","text":"` + strings.Repeat("x", 2900) + `"}]}}` + "\n")
	}
	b.WriteString(`{"type":"message","timestamp":"2026-10-15T10:05:00Z","message":{"role":"user","content":[{"type":"text","text":"LAST-PROMPT"}]}}` + "\n")
	os.WriteFile(p, []byte(b.String()), 0o644)
	s := &session.Session{Agent: session.Pi, ID: "s1", Path: p, Cwd: "/w", Title: "demo"}
	tr := Transcript([]*session.Session{s})
	if !strings.Contains(tr, "USER: LAST-PROMPT") || !strings.Contains(tr, "===== SESSION: demo") {
		t.Fatal("missing latest prompt or session header")
	}
	if strings.Contains(tr, "FIRST-PROMPT") || !strings.Contains(tr, "earlier conversation omitted") {
		t.Fatal("over-budget transcript should keep only the tail")
	}
	if len(tr) > maxTranscript+2000 {
		t.Fatalf("transcript too long: %d", len(tr))
	}
}

func TestStartCommandQuotesPath(t *testing.T) {
	got := StartCommand(session.Codex, "/Users/me/it's/note.md")
	want := `codex "$(cat '/Users/me/it'\''s/note.md')"`
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}
