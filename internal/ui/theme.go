package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

// Theme is a complete palette; applyTheme turns it into the package-level colors and styles.
type Theme struct {
	Name                       string
	Accent, Accent2            lipgloss.Color // brand gradient (logo, active tab, focus)
	Text, Muted, Faint, Border lipgloss.Color
	Sel                        lipgloss.Color // selection / surface background
	OnAccent                   lipgloss.Color // text drawn on accent backgrounds
	Claude, Codex, Pi          lipgloss.Color
	Green, Yellow, Red, Code   lipgloss.Color
}

var themes = []Theme{
	{
		Name: "midnight", Accent: "#22D3EE", Accent2: "#C084FC",
		Text: "#E6E9F5", Muted: "#7C86A8", Faint: "#343B5C", Border: "#2B3254", Sel: "#1B2140", OnAccent: "#0B0E1A",
		Claude: "#FF8A65", Codex: "#34D399", Pi: "#A78BFA",
		Green: "#4ADE80", Yellow: "#FBBF24", Red: "#F87171", Code: "#93C5FD",
	},
	{
		Name: "catppuccin", Accent: "#CBA6F7", Accent2: "#F5C2E7",
		Text: "#CDD6F4", Muted: "#9399B2", Faint: "#45475A", Border: "#585B70", Sel: "#313244", OnAccent: "#1E1E2E",
		Claude: "#FAB387", Codex: "#A6E3A1", Pi: "#B4BEFE",
		Green: "#A6E3A1", Yellow: "#F9E2AF", Red: "#F38BA8", Code: "#89DCEB",
	},
	{
		Name: "mono", Accent: "#FBBF24", Accent2: "#F97316",
		Text: "#E5E5E5", Muted: "#8A8A8A", Faint: "#3A3A3A", Border: "#404040", Sel: "#262626", OnAccent: "#0A0A0A",
		Claude: "#E5E5E5", Codex: "#BDBDBD", Pi: "#9E9E9E",
		Green: "#BEF264", Yellow: "#FBBF24", Red: "#F87171", Code: "#D4D4D4",
	},
	{
		Name: "retro", Accent: "#39FF14", Accent2: "#FFB000",
		Text: "#C8FFC0", Muted: "#4FA84F", Faint: "#1E3A1E", Border: "#2E6B2E", Sel: "#0E2A0E", OnAccent: "#001100",
		Claude: "#FFB000", Codex: "#39FF14", Pi: "#7CFFCB",
		Green: "#39FF14", Yellow: "#FFB000", Red: "#FF5555", Code: "#7CFFCB",
	},
}

func themeByName(name string) (Theme, int) {
	for i, t := range themes {
		if strings.EqualFold(t.Name, name) {
			return t, i
		}
	}
	return themes[0], 0
}

var (
	theme Theme

	colClaude, colCodex, colPi                       lipgloss.Color
	colAccent, colAccent2, colDim, colFaint, colText lipgloss.Color
	colGreen, colYellow, colRed, colSelBg, colCode   lipgloss.Color
	colBorder, colOnAccent                           lipgloss.Color

	stDim, stFaint, stBold, stText, stAccent, stAccent2 lipgloss.Style
	stGreen, stYellow, stRed, stCode, stKey, stSection  lipgloss.Style
	stTitle, stTabOn, stTabOff                          lipgloss.Style
	stPane, stPaneOn, stModal                           lipgloss.Style
)

func init() { applyTheme(themes[0]) }

func applyTheme(t Theme) {
	theme = t
	colClaude, colCodex, colPi = t.Claude, t.Codex, t.Pi
	colAccent, colAccent2, colDim, colFaint, colText = t.Accent, t.Accent2, t.Muted, t.Faint, t.Text
	colGreen, colYellow, colRed, colSelBg, colCode = t.Green, t.Yellow, t.Red, t.Sel, t.Code
	colBorder, colOnAccent = t.Border, t.OnAccent

	fg := func(c lipgloss.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	stDim, stFaint, stText = fg(t.Muted), fg(t.Faint), fg(t.Text)
	stBold = fg(t.Text).Bold(true)
	stAccent, stAccent2 = fg(t.Accent), fg(t.Accent2)
	stGreen, stYellow, stRed, stCode = fg(t.Green), fg(t.Yellow), fg(t.Red), fg(t.Code)
	stKey = fg(t.Accent).Bold(true)
	stSection = fg(t.Accent).Bold(true)
	stTitle = lipgloss.NewStyle().Bold(true).Foreground(t.OnAccent).Background(t.Accent).Padding(0, 1)
	stTabOn = lipgloss.NewStyle().Bold(true).Foreground(t.OnAccent).Background(t.Accent).Padding(0, 1)
	stTabOff = fg(t.Muted).Padding(0, 1)
	stPane = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Border)
	stPaneOn = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Accent)
	stModal = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Accent).Padding(1, 2)
}

// blend mixes two hex colors (t in 0..1).
func blend(a, b lipgloss.Color, t float64) lipgloss.Color {
	ca, err1 := colorful.Hex(string(a))
	cb, err2 := colorful.Hex(string(b))
	if err1 != nil || err2 != nil {
		return a
	}
	return lipgloss.Color(ca.BlendLab(cb, t).Clamped().Hex())
}

// gradient colors each rune of s along from→to; phase (0..1) shifts the gradient for a shimmer.
func gradient(s string, from, to lipgloss.Color, bold bool, phase float64) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return ""
	}
	var b strings.Builder
	for i, r := range runes {
		if r == ' ' {
			b.WriteRune(r)
			continue
		}
		p := float64(i) / float64(max(1, len(runes)-1))
		if phase != 0 {
			// Shimmer: a triangle wave over the shifted position, so a moving phase never jumps.
			p += phase
			p -= float64(int(p))
			p = 1 - 2*abs(p-0.5)
		}
		st := lipgloss.NewStyle().Foreground(blend(from, to, p))
		if bold {
			st = st.Bold(true)
		}
		b.WriteString(st.Render(string(r)))
	}
	return b.String()
}

// pill renders text on a colored background.
func pill(text string, bg lipgloss.Color) string {
	return lipgloss.NewStyle().Bold(true).Foreground(colOnAccent).Background(bg).Padding(0, 1).Render(text)
}
