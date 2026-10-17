package term

import (
	"os"
	"strings"
	"testing"
	"time"
)

// Runs against a private tmux server: DECK_TMUX_IT=1 go test ./internal/term -run TestTmuxBackend -v
func TestTmuxBackend(t *testing.T) {
	if os.Getenv("DECK_TMUX_IT") == "" || !haveBinary("tmux") {
		t.Skip("set DECK_TMUX_IT=1 (needs tmux)")
	}
	t.Setenv("TMUX_TMPDIR", t.TempDir()) // isolated server, never the user's
	t.Setenv("TMUX", "")
	defer run("tmux", "kill-server")

	b := tmux{}
	err := b.OpenTab(t.TempDir(), "echo hello-from-deck", "Claude · demo")
	if !IsNotice(err) {
		t.Fatalf("outside tmux OpenTab should report a notice, got %v", err)
	}
	out, err := run("tmux", "list-panes", "-a", "-F", "#{pane_tty} #{window_name}")
	if err != nil {
		t.Fatal(err)
	}
	var tty string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasSuffix(l, "Claude · demo") {
			tty = strings.Fields(l)[0]
		}
	}
	if tty == "" {
		t.Fatalf("new window not found:\n%s", out)
	}
	waitFor(t, tty, "hello-from-deck")

	if ok, err := b.TypeInto(strings.TrimPrefix(tty, "/dev/"), "typed-not-sent"); !ok || err != nil {
		t.Fatalf("TypeInto = %v, %v", ok, err)
	}
	waitFor(t, tty, "typed-not-sent")
	if ok, _ := b.Focus("pts/does-not-exist"); ok {
		t.Fatal("focus on unknown tty should fail")
	}
}

func waitFor(t *testing.T, tty, text string) {
	t.Helper()
	pane, _ := paneFor(tty)
	for i := 0; i < 50; i++ {
		out, _ := run("tmux", "capture-pane", "-p", "-t", pane)
		if strings.Contains(out, text) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	out, _ := run("tmux", "capture-pane", "-p", "-t", pane)
	t.Fatalf("%q never appeared in pane:\n%s", text, out)
}
