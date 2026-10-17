package ui

import (
	"reflect"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestFuzzyMatch(t *testing.T) {
	cases := []struct {
		pat, text string
		ok        bool
	}{
		{"chk", "fix flaky checkout tests", true},
		{"flky", "fix flaky checkout tests", true},
		{"oauth", "migrate auth to OAuth", true},
		{"xyz", "fix flaky checkout tests", false},
		{"fixf", "fix flaky checkout tests", true},
		{"ft", "fix flaky checkout tests", false},  // letters too far apart: noise, not a match
		{"fxs", "fix flaky checkout tests", false}, // ditto
	}
	for _, c := range cases {
		if _, _, ok := fuzzyMatch(c.pat, c.text); ok != c.ok {
			t.Errorf("fuzzyMatch(%q, %q) ok=%v want %v", c.pat, c.text, ok, c.ok)
		}
	}
	// Word-start, consecutive matches must beat a scattered one.
	a, _, _ := fuzzyMatch("dep", "deploy codebase")
	b, _, _ := fuzzyMatch("dep", "a dashboard for each project")
	if a <= b {
		t.Errorf("expected prefix match to rank higher: %d vs %d", a, b)
	}
	_, pos, _ := fuzzyMatch("chk", "checkout")
	if !reflect.DeepEqual(pos, []int{0, 1, 4}) {
		t.Errorf("positions = %v", pos)
	}
}

func TestHighlight(t *testing.T) {
	plain := lipgloss.NewStyle()
	got := highlight("abc", []int{1}, plain, plain.SetString("").Inline(true))
	if ansi.Strip(got) != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := highlight("abc", nil, plain, plain); got != "abc" {
		t.Fatalf("got %q", got)
	}
}
