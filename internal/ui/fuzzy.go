package ui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// fuzzyMatch finds pattern's runes in order inside text (case-insensitive) and scores the match:
// consecutive runs, word starts and an early start score higher. Scattered matches are rejected so
// short queries don't match everything. positions are rune indexes into text, for highlighting.
func fuzzyMatch(pattern, text string) (score int, positions []int, ok bool) {
	p := []rune(strings.ToLower(pattern))
	t := []rune(text)
	lt := []rune(strings.ToLower(text))
	if len(p) == 0 {
		return 0, nil, true
	}
	best, found := 0, false
	var bestPos []int
	// Try each occurrence of the first rune as a starting point; keep the best-scoring alignment.
	for start := 0; start < len(lt); start++ {
		if lt[start] != p[0] {
			continue
		}
		pos := []int{start}
		j := 1
		for i := start + 1; i < len(lt) && j < len(p); i++ {
			if lt[i] == p[j] {
				pos = append(pos, i)
				j++
			}
		}
		if j < len(p) {
			break // later starts can't match either
		}
		span := pos[len(pos)-1] - pos[0] + 1
		if span > len(p)*3+2 {
			continue
		}
		s := 0
		for k, i := range pos {
			s += 1
			if k > 0 && pos[k-1] == i-1 {
				s += 6 // consecutive
			}
			if i == 0 || !unicode.IsLetter(t[i-1]) && !unicode.IsDigit(t[i-1]) {
				s += 8 // word start
			}
		}
		s -= span - len(p) // gaps
		s -= pos[0] / 4    // prefer early matches
		if !found || s > best {
			best, bestPos, found = s, pos, true
		}
	}
	return best, bestPos, found
}

// highlight renders text with the runes at positions in the highlight style and the rest in base.
func highlight(text string, positions []int, base, hl lipgloss.Style) string {
	if len(positions) == 0 {
		return base.Render(text)
	}
	set := map[int]bool{}
	for _, p := range positions {
		set[p] = true
	}
	var b, run strings.Builder
	inHL := false
	flush := func() {
		if run.Len() == 0 {
			return
		}
		if inHL {
			b.WriteString(hl.Render(run.String()))
		} else {
			b.WriteString(base.Render(run.String()))
		}
		run.Reset()
	}
	for i, r := range []rune(text) {
		if set[i] != inHL {
			flush()
			inHL = set[i]
		}
		run.WriteRune(r)
	}
	flush()
	return b.String()
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
