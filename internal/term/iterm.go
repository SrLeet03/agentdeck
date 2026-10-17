package term

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ---- iTerm2 -------------------------------------------------------------------

type iTerm struct{}

const itermNewTab = `on run argv
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

// itermFind selects the session on tty item 1; when item 2 is non-empty it is typed without a newline.
const itermFind = `on run argv
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
						if txt is not "" then tell s to write text txt newline NO
						activate
						return "ok"
					end if
				end repeat
			end repeat
		end repeat
	end tell
	return "missing"
end run`

func (iTerm) Name() string { return "iTerm2" }

func (iTerm) OpenTab(dir, command, title string) error {
	_, err := osascript(itermNewTab, shellLine(dir, command), title)
	return err
}

func (iTerm) Focus(tty string) (bool, error) {
	out, err := osascript(itermFind, devTTY(tty), "")
	return out == "ok", err
}

func (iTerm) TypeInto(tty, text string) (bool, error) {
	out, err := osascript(itermFind, devTTY(tty), text)
	return out == "ok", err
}

func (b iTerm) Check() error {
	if err := compileScripts(itermNewTab, itermFind); err != nil {
		return err
	}
	_, err := b.Focus("ttys-agentdeck-check")
	return err
}

// ---- Terminal.app ---------------------------------------------------------------

type appleTerminal struct{}

// Terminal.app has no "new tab" scripting verb, so each session gets its own window.
const terminalNew = `on run argv
	set cmd to item 1 of argv
	set tabTitle to item 2 of argv
	tell application "Terminal"
		activate
		set t to do script cmd
		if tabTitle is not "" then set custom title of t to tabTitle
	end tell
end run`

const terminalFind = `on run argv
	set target to item 1 of argv
	tell application "Terminal"
		repeat with w in windows
			repeat with t in tabs of w
				if tty of t is target then
					set selected of t to true
					set index of w to 1
					activate
					return "ok"
				end if
			end repeat
		end repeat
	end tell
	return "missing"
end run`

func (appleTerminal) Name() string { return "Terminal" }

func (appleTerminal) OpenTab(dir, command, title string) error {
	_, err := osascript(terminalNew, shellLine(dir, command), title)
	return err
}

func (appleTerminal) Focus(tty string) (bool, error) {
	out, err := osascript(terminalFind, devTTY(tty))
	return out == "ok", err
}

// TypeInto is unsupported: Terminal's `do script` always presses return. Callers fall back to the clipboard.
func (appleTerminal) TypeInto(string, string) (bool, error) { return false, nil }

func (b appleTerminal) Check() error {
	if err := compileScripts(terminalNew, terminalFind); err != nil {
		return err
	}
	_, err := b.Focus("ttys-agentdeck-check")
	return err
}

// ---- AppleScript helpers -----------------------------------------------------------

func osascript(script string, args ...string) (string, error) {
	out, err := exec.Command("osascript", append([]string{"-e", script}, args...)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("osascript: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func compileScripts(scripts ...string) error {
	tmp, err := os.MkdirTemp("", "agentdeck")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for i, sc := range scripts {
		out, err := exec.Command("osacompile", "-e", sc, "-o", filepath.Join(tmp, fmt.Sprintf("s%d.scpt", i))).CombinedOutput()
		if err != nil {
			return fmt.Errorf("script %d does not compile: %s", i, strings.TrimSpace(string(out)))
		}
	}
	return nil
}
