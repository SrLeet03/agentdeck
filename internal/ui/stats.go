package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/SrLeet03/agentdeck/internal/session"
	"github.com/charmbracelet/lipgloss"
)

var agents = []session.Agent{session.Claude, session.Codex, session.Pi}

func (m Model) viewStats() string {
	w := m.w - 4
	h := m.bodyHeight() - 2
	colW := w
	twoCol := w >= 110
	if twoCol {
		colW = (w - 3) / 2
	}

	// per-agent totals
	type agg struct {
		sessions, live int
		tok            session.Tokens
	}
	per := map[session.Agent]*agg{}
	for _, a := range agents {
		per[a] = &agg{}
	}
	for _, s := range m.sessions {
		g := per[s.Agent]
		g.sessions++
		g.tok.Add(s.Tokens)
		if m.liveBy[s.Key()] != nil {
			g.live++
		}
	}
	left := []string{stSection.Render("Agents (all time)"), ""}
	hdr := fmt.Sprintf("%-12s %8s %5s %9s %8s %8s %9s", "", "sessions", "live", "total", "in", "out", "cache")
	left = append(left, stDim.Render(trunc(hdr, colW)))
	for _, a := range agents {
		g := per[a]
		line := lipgloss.NewStyle().Foreground(agentColor(a)).Bold(true).Render(fmt.Sprintf("%-12s", agentName(a))) +
			fmt.Sprintf(" %8d %5d %9s %8s %8s %9s", g.sessions, g.live, humanTokens(g.tok.Total()),
				humanTokens(g.tok.In), humanTokens(g.tok.Out), humanTokens(g.tok.CacheRead+g.tok.CacheWrite))
		left = append(left, trunc(line, colW))
	}

	// top projects, last 30 days
	cut := time.Now().AddDate(0, 0, -29).Format("2006-01-02")
	type proj struct {
		key      string
		sessions int
		tok      int64
		last     time.Time
		by       map[session.Agent]int64
	}
	projs := map[string]*proj{}
	for _, s := range m.sessions {
		var t int64
		for d, n := range s.Daily {
			if d >= cut {
				t += n
			}
		}
		if t == 0 && m.activity(s).Format("2006-01-02") < cut {
			continue
		}
		k := projectKey(s.Cwd)
		p := projs[k]
		if p == nil {
			p = &proj{key: k, by: map[session.Agent]int64{}}
			projs[k] = p
		}
		p.sessions++
		p.tok += t
		p.by[s.Agent] += t
		if a := m.activity(s); a.After(p.last) {
			p.last = a
		}
	}
	var plist []*proj
	for _, p := range projs {
		plist = append(plist, p)
	}
	sort.Slice(plist, func(i, j int) bool { return plist[i].tok > plist[j].tok })
	left = append(left, "", stSection.Render("Top projects (last 30 days)"), "")
	nameW := max(12, colW-30)
	left = append(left, stDim.Render(trunc(fmt.Sprintf("%-*s %5s %9s %6s", nameW, "project", "sess", "tokens", "last"), colW)))
	room := h - len(left) - 1
	if !twoCol {
		room = min(room, 8)
	}
	for i, p := range plist {
		if i >= room {
			break
		}
		left = append(left, trunc(fmt.Sprintf("%-*s %5d %9s %6s", nameW, trunc(session.ShortPath(p.key), nameW), p.sessions, humanTokens(p.tok), ago(p.last)), colW))
	}

	// daily chart
	days := 14
	right := []string{stSection.Render("Tokens per day (last 14 days)") + "  " +
		lipgloss.NewStyle().Foreground(colClaude).Render("■ CC ") + lipgloss.NewStyle().Foreground(colCodex).Render("■ CX ") + lipgloss.NewStyle().Foreground(colPi).Render("■ PI"), ""}
	type day struct {
		label string
		by    map[session.Agent]int64
		total int64
	}
	var ds []day
	var maxT int64
	for i := days - 1; i >= 0; i-- {
		d := time.Now().AddDate(0, 0, -i)
		k := d.Format("2006-01-02")
		dd := day{label: d.Format("Mon Jan 02"), by: map[session.Agent]int64{}}
		for _, s := range m.sessions {
			if n := s.Daily[k]; n > 0 {
				dd.by[s.Agent] += n
				dd.total += n
			}
		}
		maxT = max(maxT, dd.total)
		ds = append(ds, dd)
	}
	barW := max(10, colW-21)
	for _, d := range ds {
		var bar string
		used := 0
		for _, a := range agents {
			if maxT == 0 || d.by[a] == 0 {
				continue
			}
			n := int(float64(d.by[a]) / float64(maxT) * float64(barW))
			if n == 0 {
				n = 1
			}
			used += n
			bar += lipgloss.NewStyle().Foreground(agentColor(a)).Render(strings.Repeat("█", n))
		}
		bar += strings.Repeat(" ", max(0, barW-used))
		right = append(right, stDim.Render(d.label)+" "+bar+" "+fmt.Sprintf("%6s", humanTokens(d.total)))
	}
	right = append(right, "", stDim.Render(trunc("Tokens include cache reads/writes. Claude totals include its subagents.", colW)))

	var body string
	if twoCol {
		body = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(colW).Render(strings.Join(left, "\n")), "   ",
			lipgloss.NewStyle().Width(colW).Render(strings.Join(right, "\n")))
	} else {
		body = strings.Join(append(append(left, ""), right...), "\n")
	}
	return stPaneOn.Width(m.w-2).Height(h).Padding(0, 1).Render(body)
}
