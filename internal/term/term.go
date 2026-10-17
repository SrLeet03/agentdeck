// Package term opens, focuses and types into terminal tabs/windows: iTerm2 and Terminal.app on macOS,
// tmux anywhere, and GNOME Terminal / Konsole on Linux.
package term

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/SrLeet03/agentdeck/internal/config"
)

// Backend drives one terminal emulator (or multiplexer).
type Backend interface {
	// Name is shown to the user, e.g. "iTerm2".
	Name() string
	// OpenTab runs command (a shell line) in a new tab or window, after cd-ing into dir.
	OpenTab(dir, command, title string) error
	// Focus brings the tab owning tty (e.g. "ttys003", "pts/4") to the front; false if not found or unsupported.
	Focus(tty string) (bool, error)
	// TypeInto types text into the tab owning tty without pressing enter; false if unsupported.
	TypeInto(tty, text string) (bool, error)
	// Check verifies the backend can be driven (permissions, binaries).
	Check() error
}

// Notice is an informational outcome reported through the error channel (e.g. "opened in detached tmux").
type Notice string

func (n Notice) Error() string { return string(n) }

// IsNotice reports whether err is informational rather than a failure.
func IsNotice(err error) bool {
	var n Notice
	return errors.As(err, &n)
}

// Names accepted in config.json's "terminal" field.
var Names = []string{"auto", "iterm2", "terminal", "tmux", "gnome-terminal", "konsole", "none"}

var (
	once    sync.Once
	current Backend
)

// Current returns the backend chosen by config ("terminal") or auto-detection.
func Current() Backend {
	once.Do(func() { current = choose(config.Get().Terminal, os.Getenv, runtime.GOOS, haveBinary, haveApp) })
	return current
}

func haveBinary(name string) bool { _, err := exec.LookPath(name); return err == nil }

func haveApp(path string) bool { _, err := os.Stat(path); return err == nil }

// choose picks a backend; its inputs are injected so detection can be tested.
func choose(want string, env func(string) string, goos string, bin func(string) bool, app func(string) bool) Backend {
	switch strings.ToLower(strings.TrimSpace(want)) {
	case "iterm2", "iterm":
		return iTerm{}
	case "terminal", "terminal.app", "apple_terminal":
		return appleTerminal{}
	case "tmux":
		return tmux{}
	case "gnome-terminal", "gnome":
		return gnome{}
	case "konsole":
		return konsole{}
	case "none", "clipboard":
		return clipboardOnly{}
	}
	// auto: prefer the terminal deck itself is running in.
	if env("TMUX") != "" {
		return tmux{}
	}
	switch env("TERM_PROGRAM") {
	case "iTerm.app":
		return iTerm{}
	case "Apple_Terminal":
		return appleTerminal{}
	}
	if goos == "linux" {
		if env("KONSOLE_VERSION") != "" && bin("konsole") {
			return konsole{}
		}
		if (env("GNOME_TERMINAL_SCREEN") != "" || env("GNOME_TERMINAL_SERVICE") != "") && bin("gnome-terminal") {
			return gnome{}
		}
		if bin("gnome-terminal") {
			return gnome{}
		}
		if bin("konsole") {
			return konsole{}
		}
		if bin("tmux") {
			return tmux{}
		}
		return clipboardOnly{}
	}
	if goos == "darwin" {
		if app("/Applications/iTerm.app") {
			return iTerm{}
		}
		return appleTerminal{}
	}
	if bin("tmux") {
		return tmux{}
	}
	return clipboardOnly{}
}

// ---- package-level helpers used by the UI --------------------------------------

func Name() string { return Current().Name() }

func OpenTab(dir, command, title string) error { return Current().OpenTab(dir, command, title) }

func Focus(tty string) (bool, error) {
	if tty == "" {
		return false, nil
	}
	return Current().Focus(tty)
}

func TypeInto(tty, text string) (bool, error) {
	if tty == "" {
		return false, nil
	}
	return Current().TypeInto(tty, text)
}

func Check() error { return Current().Check() }

// shellLine is what gets typed or executed in the new tab.
func shellLine(dir, command string) string {
	if dir == "" {
		return command
	}
	return "cd " + Quote(dir) + " && " + command
}

func devTTY(tty string) string {
	if strings.HasPrefix(tty, "/dev/") {
		return tty
	}
	return "/dev/" + tty
}

// userShell is the login shell used to run commands in terminals that take an argv instead of typed text.
func userShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/sh"
}

// Quote single-quotes s for sh/bash/zsh.
func Quote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./~+@:", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ExistingDir returns dir, or its nearest existing ancestor when dir was deleted (e.g. a removed worktree).
func ExistingDir(dir string) (string, bool) {
	if dir == "" {
		h, _ := os.UserHomeDir()
		return h, false
	}
	d := dir
	for {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d, d == dir
		}
		parent := filepath.Dir(d)
		if parent == d {
			h, _ := os.UserHomeDir()
			return h, false
		}
		d = parent
	}
}

// CopyToClipboard puts s on the system clipboard (pbcopy, wl-copy, xclip or xsel).
func CopyToClipboard(s string) error {
	var cmds [][]string
	if runtime.GOOS == "darwin" {
		cmds = [][]string{{"pbcopy"}}
	} else {
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			cmds = append(cmds, []string{"wl-copy"})
		}
		cmds = append(cmds, []string{"xclip", "-selection", "clipboard"}, []string{"xsel", "--clipboard", "--input"})
		if os.Getenv("TMUX") != "" {
			cmds = append(cmds, []string{"tmux", "load-buffer", "-"})
		}
	}
	for _, c := range cmds {
		if !haveBinary(c[0]) {
			continue
		}
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Stdin = strings.NewReader(s)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no clipboard tool found (install wl-clipboard, xclip or xsel)")
}

func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %s", name, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
