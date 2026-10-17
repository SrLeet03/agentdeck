package live

import (
	"testing"
	"time"
)

func TestAgentOf(t *testing.T) {
	cases := map[string]string{
		"codex":                                 "codex",
		"/Users/x/.local/bin/codex resume 01a0": "codex",
		"/x/codex -c features.code_mode_host=true app-server --analytics-default-enabled": "",
		"node /Users/x/.fnm/bin/pi --session foo.jsonl":                                   "pi",
		"pi":                                     "pi",
		"pi --export s.jsonl":                    "",
		"claude bg-spare --bg-spare /tmp/x.sock": "",
		"vim pi.go":                              "",
		"":                                       "",
	}
	for in, want := range cases {
		if got := agentOf(in); got != want {
			t.Errorf("agentOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseEtime(t *testing.T) {
	cases := map[string]time.Duration{
		"05:03":      5*time.Minute + 3*time.Second,
		"02:05:03":   2*time.Hour + 5*time.Minute + 3*time.Second,
		"3-02:05:03": 3*24*time.Hour + 2*time.Hour + 5*time.Minute + 3*time.Second,
	}
	for in, want := range cases {
		if got := parseEtime(in); got != want {
			t.Errorf("parseEtime(%q) = %v, want %v", in, got, want)
		}
	}
}
