package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	reBold   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reCode   = regexp.MustCompile("`([^`]+)`")
	reLink   = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	reBullet = regexp.MustCompile(`^(\s*)[-*+]\s+`)
	reNumber = regexp.MustCompile(`^(\s*)(\d+)[.)]\s+`)
)

// renderMarkdown renders a light subset of markdown (headings, lists, fences, bold, inline code, links)
// into width-wrapped, styled lines. base styles ordinary text.
func renderMarkdown(text string, width int, base lipgloss.Style) []string {
	text = clean(text)
	var out []string
	inFence := false
	lang := ""
	for _, raw := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "```") {
			if !inFence {
				inFence, lang = true, strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
				label := "code"
				if lang != "" {
					label = lang
				}
				out = append(out, stFaint.Render("╭─ ")+stDim.Render(label))
			} else {
				inFence = false
				out = append(out, stFaint.Render("╰─"))
			}
			continue
		}
		if inFence {
			line := strings.ReplaceAll(raw, "\t", "  ")
			out = append(out, stFaint.Render("│ ")+stCode.Render(ansi.Truncate(line, max(1, width-2), "…")))
			continue
		}
		if trimmed == "" {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		// Headings.
		if strings.HasPrefix(trimmed, "#") {
			h := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			for _, l := range wrapPlain(h, width) {
				out = append(out, stAccent.Bold(true).Render(l))
			}
			continue
		}
		// Horizontal rule.
		if trimmed == "---" || trimmed == "***" {
			out = append(out, stFaint.Render(strings.Repeat("─", min(width, 24))))
			continue
		}
		// Lists keep a hanging indent.
		prefix, rest := "", raw
		if m := reBullet.FindStringSubmatch(raw); m != nil {
			prefix = m[1] + stAccent.Render("•") + " "
			rest = raw[len(m[0]):]
		} else if m := reNumber.FindStringSubmatch(raw); m != nil {
			prefix = m[1] + stAccent.Render(m[2]+".") + " "
			rest = raw[len(m[0]):]
		}
		// Quotes.
		if strings.HasPrefix(trimmed, ">") {
			prefix = stFaint.Render("▏ ")
			rest = strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
		}
		pw := lipgloss.Width(prefix)
		for i, l := range wrapPlain(strings.TrimSpace(rest), max(8, width-pw)) {
			p := prefix
			if i > 0 {
				p = strings.Repeat(" ", pw)
			}
			out = append(out, p+inline(l, base))
		}
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// inline styles **bold**, `code` and [links](url) within one already-wrapped line.
func inline(line string, base lipgloss.Style) string {
	type span struct {
		start, end int
		render     string
	}
	var spans []span
	for _, m := range reCode.FindAllStringSubmatchIndex(line, -1) {
		spans = append(spans, span{m[0], m[1], stCode.Render(line[m[2]:m[3]])})
	}
	overlaps := func(a, b int) bool {
		for _, s := range spans {
			if a < s.end && b > s.start {
				return true
			}
		}
		return false
	}
	for _, m := range reBold.FindAllStringSubmatchIndex(line, -1) {
		if !overlaps(m[0], m[1]) {
			spans = append(spans, span{m[0], m[1], base.Bold(true).Render(line[m[2]:m[3]])})
		}
	}
	for _, m := range reLink.FindAllStringSubmatchIndex(line, -1) {
		if !overlaps(m[0], m[1]) {
			spans = append(spans, span{m[0], m[1], stAccent.Underline(true).Render(line[m[2]:m[3]])})
		}
	}
	if len(spans) == 0 {
		return base.Render(line)
	}
	// Sort spans by start (few spans; insertion sort).
	for i := 1; i < len(spans); i++ {
		for j := i; j > 0 && spans[j].start < spans[j-1].start; j-- {
			spans[j], spans[j-1] = spans[j-1], spans[j]
		}
	}
	var b strings.Builder
	pos := 0
	for _, s := range spans {
		if s.start < pos {
			continue
		}
		b.WriteString(base.Render(line[pos:s.start]))
		b.WriteString(s.render)
		pos = s.end
	}
	b.WriteString(base.Render(line[pos:]))
	return b.String()
}

// wrapPlain word-wraps unstyled text to width.
func wrapPlain(s string, width int) []string {
	if width <= 0 {
		return nil
	}
	return strings.Split(lipgloss.NewStyle().Width(width).Render(s), "\n")
}
