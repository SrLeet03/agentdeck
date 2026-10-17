package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// overlay draws box centered over bg (w×h). With dim, the background is recolored faint so the modal
// stands out, like a backdrop.
func overlay(bg, box string, w, h int, dim bool) string {
	lines := strings.Split(bg, "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}
	if dim {
		for i, l := range lines {
			lines[i] = stFaint.Render(ansi.Strip(l))
		}
	}
	boxLines := strings.Split(box, "\n")
	bw := lipgloss.Width(box)
	x := max(0, (w-bw)/2)
	y := max(0, (h-len(boxLines))/2)
	for i, bl := range boxLines {
		row := y + i
		if row >= len(lines) {
			break
		}
		left := ansi.Truncate(lines[row], x, "")
		if lw := lipgloss.Width(left); lw < x {
			left += strings.Repeat(" ", x-lw)
		}
		right := ansi.TruncateLeft(lines[row], x+bw, "")
		lines[row] = left + bl + right
	}
	return strings.Join(lines, "\n")
}
