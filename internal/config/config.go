// Package config holds user settings from ~/.agentdeck/config.json.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Config struct {
	// Env sets environment variables per agent ("claude", "codex", "pi") for every launch:
	// commands typed into iTerm2 tabs get an `export K=V && ` prefix, and commands deck runs itself get them in their env.
	Env map[string]map[string]string `json:"env"`
	// Terminal picks where sessions open: auto (default), iterm2, terminal, tmux, gnome-terminal, konsole or none.
	Terminal string `json:"terminal,omitempty"`
	// Theme is the dashboard color theme (midnight, catppuccin, mono, retro).
	Theme string `json:"theme,omitempty"`
}

// SetTheme remembers the chosen theme, keeping every other setting in the file.
func SetTheme(name string) error {
	c := Get()
	c.Theme = name
	current = c
	out, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := Path() + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, Path())
}

// Default has no overrides; agents launch with the shell's own environment.
func Default() Config {
	return Config{Env: map[string]map[string]string{}}
}

var (
	once    sync.Once
	current Config
)

// Path is the config file location.
func Path() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agentdeck", "config.json")
}

// Get loads the config once, writing the default file the first time so it can be edited.
func Get() Config {
	once.Do(func() {
		current = Default()
		b, err := os.ReadFile(Path())
		if os.IsNotExist(err) {
			os.MkdirAll(filepath.Dir(Path()), 0o755)
			if out, err := json.MarshalIndent(current, "", "  "); err == nil {
				os.WriteFile(Path(), append(out, '\n'), 0o644)
			}
			return
		}
		var c Config
		if err == nil && json.Unmarshal(b, &c) == nil {
			if c.Env == nil {
				c.Env = map[string]map[string]string{}
			}
			current = c
		}
	})
	return current
}

func (c Config) vars(agent string) [][2]string {
	m := c.Env[agent]
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([][2]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, [2]string{k, m[k]})
	}
	return out
}

// Command prefixes a shell command line for agent with its configured exports.
func (c Config) Command(agent, command string) string {
	var exports []string
	for _, kv := range c.vars(agent) {
		exports = append(exports, "export "+kv[0]+"="+shellQuote(kv[1]))
	}
	if len(exports) == 0 {
		return command
	}
	return strings.Join(exports, " && ") + " && " + command
}

// Environ returns the current environment with agent's overrides applied, for exec.Cmd.Env.
func (c Config) Environ(agent string) []string {
	over := c.vars(agent)
	if len(over) == 0 {
		return os.Environ()
	}
	skip := map[string]bool{}
	for _, kv := range over {
		skip[kv[0]] = true
	}
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if !skip[k] {
			env = append(env, kv)
		}
	}
	for _, kv := range over {
		env = append(env, kv[0]+"="+kv[1])
	}
	return env
}

func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./:@", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
