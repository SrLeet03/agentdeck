// Package term opens and focuses iTerm2 tabs via AppleScript.
package term

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const newTabScript = `on run argv
	set cmd to item 1 of argv
	set tabTitle to item 2 of argv
	tell application "iTerm2"
		activate
		if (count of windows) = 0 then
			set w to (create window with default profile)
		else
			set w to current window
			tell w to create tab with default profile
		end if
		tell current session of w
			if tabTitle is not "" then set name to tabTitle
			write text cmd
		end tell
	end tell
end run`

const focusScript = `on run argv
	set target to item 1 of argv
	tell application "iTerm2"
		repeat with w in windows
			repeat with t in tabs of w
				repeat with s in sessions of t
					if tty of s is target then
						select w
						tell t to select
						tell s to select
						activate
						return "ok"
					end if
				end repeat
			end repeat
		end repeat
	end tell
	return "missing"
end run`

const typeScript = `on run argv
	set target to item 1 of argv
	set txt to item 2 of argv
	tell application "iTerm2"
		repeat with w in windows
			repeat with t in tabs of w
				repeat with s in sessions of t
					if tty of s is target then
						select w
						tell t to select
						tell s to select
						tell s to write text txt newline NO
						activate
						return "ok"
					end if
				end repeat
			end repeat
		end repeat
	end tell
	return "missing"
end run`

// TypeInto types text into the iTerm2 session on tty without pressing enter, and focuses it.
func TypeInto(tty, text string) (bool, error) {
	if tty == "" {
		return false, nil
	}
	if !strings.HasPrefix(tty, "/dev/") {
		tty = "/dev/" + tty
	}
	out, err := osascript(typeScript, tty, text)
	return out == "ok", err
}

func osascript(script string, args ...string) (string, error) {
	cmd := exec.Command("osascript", append([]string{"-e", script}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("osascript: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// OpenTab runs command in a new iTerm2 tab after cd-ing into dir.
func OpenTab(dir, command, title string) error {
	line := command
	if dir != "" {
		line = "cd " + Quote(dir) + " && " + command
	}
	_, err := osascript(newTabScript, line, title)
	return err
}

// Focus brings the iTerm2 session on the given tty (e.g. "ttys003") to the front.
func Focus(tty string) (bool, error) {
	if tty == "" {
		return false, nil
	}
	if !strings.HasPrefix(tty, "/dev/") {
		tty = "/dev/" + tty
	}
	out, err := osascript(focusScript, tty)
	return out == "ok", err
}

// Quote single-quotes s for zsh/bash.
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

// CopyToClipboard puts s on the macOS clipboard.
func CopyToClipboard(s string) error {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(s)
	return cmd.Run()
}

// Check compiles both scripts and does a harmless iTerm2 lookup to confirm automation access.
func Check() error {
	tmp, err := os.MkdirTemp("", "agentdeck")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for i, sc := range []string{newTabScript, focusScript, typeScript} {
		out, err := exec.Command("osacompile", "-e", sc, "-o", filepath.Join(tmp, fmt.Sprintf("s%d.scpt", i))).CombinedOutput()
		if err != nil {
			return fmt.Errorf("script %d does not compile: %s", i, strings.TrimSpace(string(out)))
		}
	}
	_, err = Focus("ttys-agentdeck-check")
	return err
}
