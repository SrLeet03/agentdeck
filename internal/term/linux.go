package term

import (
	"fmt"
	"os/exec"
)

// GNOME Terminal and Konsole can open tabs but expose no way to find or type into an existing one,
// so Focus/TypeInto report false and the UI falls back to the clipboard.

// argvCommand runs line in the user's shell and keeps an interactive shell open afterwards.
func argvCommand(line string) []string {
	sh := userShell()
	return []string{sh, "-ic", line + "; exec " + Quote(sh)}
}

// start launches a GUI terminal without waiting for it.
func start(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	go cmd.Wait()
	return nil
}

type gnome struct{}

func (gnome) Name() string { return "GNOME Terminal" }

func (gnome) OpenTab(dir, command, title string) error {
	args := []string{"--tab"}
	if dir != "" {
		args = append(args, "--working-directory="+dir)
	}
	if title != "" {
		args = append(args, "--title="+title)
	}
	args = append(args, "--")
	return start("gnome-terminal", append(args, argvCommand(shellLine(dir, command))...)...)
}

func (gnome) Focus(string) (bool, error)            { return false, nil }
func (gnome) TypeInto(string, string) (bool, error) { return false, nil }
func (gnome) Check() error {
	if !haveBinary("gnome-terminal") {
		return fmt.Errorf("gnome-terminal is not installed")
	}
	return nil
}

type konsole struct{}

func (konsole) Name() string { return "Konsole" }

func (konsole) OpenTab(dir, command, title string) error {
	args := []string{"--new-tab"}
	if dir != "" {
		args = append(args, "--workdir", dir)
	}
	if title != "" {
		args = append(args, "-p", "tabtitle="+title)
	}
	args = append(args, "-e")
	return start("konsole", append(args, argvCommand(shellLine(dir, command))...)...)
}

func (konsole) Focus(string) (bool, error)            { return false, nil }
func (konsole) TypeInto(string, string) (bool, error) { return false, nil }
func (konsole) Check() error {
	if !haveBinary("konsole") {
		return fmt.Errorf("konsole is not installed")
	}
	return nil
}

// clipboardOnly is used when no supported terminal is found: commands go to the clipboard.
type clipboardOnly struct{}

func (clipboardOnly) Name() string { return "clipboard" }

func (clipboardOnly) OpenTab(dir, command, _ string) error {
	line := shellLine(dir, command)
	if err := CopyToClipboard(line); err != nil {
		return fmt.Errorf("no supported terminal found; run this yourself: %s", line)
	}
	return Notice("no supported terminal found — the command is on your clipboard, paste it in a new terminal")
}

func (clipboardOnly) Focus(string) (bool, error)            { return false, nil }
func (clipboardOnly) TypeInto(string, string) (bool, error) { return false, nil }
func (clipboardOnly) Check() error {
	return fmt.Errorf(`no supported terminal detected; set "terminal" in ~/.agentdeck/config.json (one of: %v)`, Names)
}
