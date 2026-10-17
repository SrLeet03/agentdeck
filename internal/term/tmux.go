package term

import (
	"fmt"
	"os"
	"strings"
)

// tmux opens each session in a new window. Outside tmux it uses (and creates) a detached session named "deck".
type tmux struct{}

const tmuxSession = "deck"

func (tmux) Name() string { return "tmux" }

func (tmux) OpenTab(dir, command, title string) error {
	args := []string{"new-window", "-P", "-F", "#{pane_id}"}
	if os.Getenv("TMUX") == "" {
		if _, err := run("tmux", "has-session", "-t", tmuxSession); err != nil {
			if _, err := run("tmux", "new-session", "-d", "-s", tmuxSession); err != nil {
				return err
			}
		}
		args = append(args, "-t", tmuxSession+":")
	}
	if dir != "" {
		args = append(args, "-c", dir)
	}
	if title != "" {
		args = append(args, "-n", windowName(title))
	}
	pane, err := run("tmux", args...)
	if err != nil {
		return err
	}
	// Type the command into the new window's shell so profile exports apply, like a typed command.
	if _, err := run("tmux", "send-keys", "-t", pane, "-l", shellLine(dir, command)); err != nil {
		return err
	}
	_, err = run("tmux", "send-keys", "-t", pane, "Enter")
	if err == nil && os.Getenv("TMUX") == "" {
		err = Notice(fmt.Sprintf("opened in tmux session %q — attach with: tmux attach -t %s", tmuxSession, tmuxSession))
	}
	return err
}

// windowName keeps tmux window names short and free of characters tmux treats specially.
func windowName(title string) string {
	title = strings.NewReplacer(".", " ", ":", " ").Replace(title)
	if r := []rune(title); len(r) > 30 {
		title = string(r[:30])
	}
	return title
}

// paneFor finds the pane whose tty is tty.
func paneFor(tty string) (string, error) {
	out, err := run("tmux", "list-panes", "-a", "-F", "#{pane_tty} #{pane_id}")
	if err != nil {
		return "", err
	}
	want := devTTY(tty)
	for _, line := range strings.Split(out, "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == want {
			return f[1], nil
		}
	}
	return "", nil
}

func (tmux) Focus(tty string) (bool, error) {
	pane, err := paneFor(tty)
	if err != nil || pane == "" {
		return false, err
	}
	if os.Getenv("TMUX") != "" {
		if _, err := run("tmux", "switch-client", "-t", pane); err != nil {
			return false, err
		}
	}
	if _, err := run("tmux", "select-window", "-t", pane); err != nil {
		return false, err
	}
	_, err = run("tmux", "select-pane", "-t", pane)
	return err == nil, err
}

func (b tmux) TypeInto(tty, text string) (bool, error) {
	pane, err := paneFor(tty)
	if err != nil || pane == "" {
		return false, err
	}
	if _, err := run("tmux", "send-keys", "-t", pane, "-l", text); err != nil {
		return false, err
	}
	return b.Focus(tty)
}

func (tmux) Check() error {
	if !haveBinary("tmux") {
		return fmt.Errorf("tmux is not installed")
	}
	return nil
}
