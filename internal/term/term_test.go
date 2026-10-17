package term

import "testing"

func TestChoose(t *testing.T) {
	envOf := func(kv map[string]string) func(string) string { return func(k string) string { return kv[k] } }
	has := func(names ...string) func(string) bool {
		return func(n string) bool {
			for _, x := range names {
				if x == n {
					return true
				}
			}
			return false
		}
	}
	none := has()
	cases := []struct {
		name, want, goos string
		env              map[string]string
		bins, apps       func(string) bool
		expect           string
	}{
		{"config wins", "konsole", "darwin", map[string]string{"TERM_PROGRAM": "iTerm.app"}, none, none, "Konsole"},
		{"inside tmux", "", "darwin", map[string]string{"TMUX": "/tmp/x", "TERM_PROGRAM": "iTerm.app"}, none, none, "tmux"},
		{"inside iTerm2", "auto", "darwin", map[string]string{"TERM_PROGRAM": "iTerm.app"}, none, none, "iTerm2"},
		{"inside Terminal.app", "", "darwin", map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, none, has("/Applications/iTerm.app"), "Terminal"},
		{"mac, iTerm2 installed", "", "darwin", nil, none, has("/Applications/iTerm.app"), "iTerm2"},
		{"mac, no iTerm2", "", "darwin", nil, none, none, "Terminal"},
		{"inside Konsole", "", "linux", map[string]string{"KONSOLE_VERSION": "230401"}, has("konsole", "gnome-terminal"), none, "Konsole"},
		{"inside GNOME", "", "linux", map[string]string{"GNOME_TERMINAL_SCREEN": "/org/x"}, has("konsole", "gnome-terminal"), none, "GNOME Terminal"},
		{"linux, unknown term, gnome installed", "", "linux", nil, has("gnome-terminal"), none, "GNOME Terminal"},
		{"linux, only tmux", "", "linux", nil, has("tmux"), none, "tmux"},
		{"linux, nothing", "", "linux", nil, none, none, "clipboard"},
	}
	for _, c := range cases {
		if got := choose(c.want, envOf(c.env), c.goos, c.bins, c.apps).Name(); got != c.expect {
			t.Errorf("%s: got %s, want %s", c.name, got, c.expect)
		}
	}
}

func TestArgvCommandKeepsShellOpen(t *testing.T) {
	t.Setenv("SHELL", "/bin/bash")
	got := argvCommand("cd /tmp && claude")
	if len(got) != 3 || got[0] != "/bin/bash" || got[1] != "-ic" || got[2] != "cd /tmp && claude; exec /bin/bash" {
		t.Fatalf("got %q", got)
	}
}
