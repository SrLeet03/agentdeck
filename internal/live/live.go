// Package live finds running Claude Code, Codex and Pi processes and maps them to session files.
package live

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Status string

const (
	Working Status = "working"
	Idle    Status = "idle" // waiting for you
	Shell   Status = "shell"
	Unknown Status = "running"
)

type Proc struct {
	Agent     string // claude | codex | pi
	PID       int
	TTY       string // e.g. ttys003, "" when detached
	Started   time.Time
	Cwd       string
	SessionID string // Claude only (from its pid file)
	JobID     string // Claude background job id, for `claude attach`
	Name      string
	Kind      string // claude: bg | interactive
	Status    Status
	File      string // session file held open (codex/pi), if found
	// AttachedTTY is a terminal tab currently attached to a Claude bg job.
	AttachedTTY string
}

type psRow struct {
	pid, ppid int
	tty       string
	elapsed   time.Duration
	args      string
}

func ps() map[int]psRow {
	// -e -ww works with both BSD (macOS) and procps (Linux) ps.
	out, err := exec.Command("ps", "-e", "-ww", "-o", "pid=,ppid=,tty=,etime=,args=").Output()
	if err != nil {
		return nil
	}
	rows := map[int]psRow{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		ppid, _ := strconv.Atoi(f[1])
		tty := f[2]
		if tty == "??" || tty == "?" { // macOS / Linux spelling of "no terminal"
			tty = ""
		}
		rows[pid] = psRow{pid, ppid, tty, parseEtime(f[3]), strings.Join(f[4:], " ")}
	}
	return rows
}

// parseEtime parses ps etime: [[dd-]hh:]mm:ss
func parseEtime(s string) time.Duration {
	var days int
	if i := strings.IndexByte(s, '-'); i >= 0 {
		days, _ = strconv.Atoi(s[:i])
		s = s[i+1:]
	}
	parts := strings.Split(s, ":")
	var secs int
	for _, p := range parts {
		n, _ := strconv.Atoi(p)
		secs = secs*60 + n
	}
	return time.Duration(days)*24*time.Hour + time.Duration(secs)*time.Second
}

// agentOf identifies interactive codex/pi CLI processes from their argv.
func agentOf(args string) string {
	f := strings.Fields(args)
	if len(f) == 0 {
		return ""
	}
	bin := filepath.Base(f[0])
	rest := f[1:]
	if bin == "node" || bin == "bun" {
		if len(f) < 2 {
			return ""
		}
		bin = filepath.Base(f[1])
		rest = f[2:]
	}
	for _, a := range rest {
		switch a {
		case "app-server", "mcp", "mcp-server", "login", "logout", "completion", "proto", "--export", "--version", "-V", "--help", "-h":
			return ""
		}
	}
	switch bin {
	case "codex", "pi":
		return bin
	}
	return ""
}

type claudePidFile struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	Status    string `json:"status"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	JobID     string `json:"jobId"`
	StartedAt int64  `json:"startedAt"`
}

// Scan returns all running agent processes.
func Scan() []Proc {
	rows := ps()
	if rows == nil {
		return nil
	}
	var procs []Proc
	now := time.Now()

	// Claude: every running CLI writes ~/.claude/sessions/<pid>.json.
	home, _ := os.UserHomeDir()
	files, _ := filepath.Glob(filepath.Join(home, ".claude", "sessions", "*.json"))
	for _, f := range files {
		pid, err := strconv.Atoi(strings.TrimSuffix(filepath.Base(f), ".json"))
		if err != nil {
			continue
		}
		r, ok := rows[pid]
		if !ok || !strings.Contains(r.args, "claude") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var pf claudePidFile
		if json.Unmarshal(b, &pf) != nil || pf.SessionID == "" {
			continue
		}
		p := Proc{Agent: "claude", PID: pid, Cwd: pf.Cwd, SessionID: pf.SessionID, JobID: pf.JobID,
			Name: pf.Name, Kind: pf.Kind, Started: now.Add(-r.elapsed)}
		if pf.StartedAt > 0 {
			p.Started = time.UnixMilli(pf.StartedAt)
		}
		if p.Kind == "" {
			p.Kind = "interactive"
		}
		if p.Kind != "bg" {
			p.TTY = r.tty
		}
		switch pf.Status {
		case "busy":
			p.Status = Working
		case "idle":
			p.Status = Idle
		case "shell":
			p.Status = Shell
		default:
			p.Status = Unknown
		}
		procs = append(procs, p)
	}

	// Terminal tabs attached to Claude background jobs (`claude attach <id>` or `claude --resume <id>`).
	for _, r := range rows {
		if r.tty == "" || !strings.Contains(r.args, "claude") {
			continue
		}
		for i := range procs {
			p := &procs[i]
			if p.Agent != "claude" || p.Kind != "bg" || p.AttachedTTY != "" || r.pid == p.PID {
				continue
			}
			if (p.JobID != "" && strings.Contains(r.args, "attach "+p.JobID)) || strings.Contains(r.args, p.SessionID) {
				p.AttachedTTY = r.tty
			}
		}
	}

	// Codex / Pi: scan argv, skipping children of another agent process (e.g. pi's own node workers).
	var pids []string
	idx := map[int]int{}
	for _, r := range rows {
		a := agentOf(r.args)
		if a == "" || r.tty == "" { // interactive CLIs always own a terminal
			continue
		}
		if parent, ok := rows[r.ppid]; ok && agentOf(parent.args) == a {
			continue
		}
		idx[r.pid] = len(procs)
		procs = append(procs, Proc{Agent: a, PID: r.pid, TTY: r.tty, Started: now.Add(-r.elapsed), Status: Unknown})
		pids = append(pids, strconv.Itoa(r.pid))
	}
	if len(pids) > 0 {
		fillOpenFiles(procs, idx, pids, home)
	}
	return procs
}

// fillOpenFiles finds each process's cwd and any session file it holds open: via /proc on Linux, one lsof call elsewhere.
func fillOpenFiles(procs []Proc, idx map[int]int, pids []string, home string) {
	if runtime.GOOS == "linux" {
		fillFromProc(procs, idx, home)
		return
	}
	out, _ := exec.Command("lsof", "-w", "-n", "-P", "-p", strings.Join(pids, ","), "-Fpfn").Output()
	codexDir := filepath.Join(home, ".codex", "sessions") + "/"
	piDir := filepath.Join(home, ".pi", "agent", "sessions") + "/"
	cur := -1
	fd := ""
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ := strconv.Atoi(line[1:])
			if i, ok := idx[pid]; ok {
				cur = i
			} else {
				cur = -1
			}
		case 'f':
			fd = line[1:]
		case 'n':
			if cur < 0 {
				continue
			}
			name := line[1:]
			if fd == "cwd" {
				procs[cur].Cwd = name
			} else if strings.HasSuffix(name, ".jsonl") && (strings.HasPrefix(name, codexDir) || strings.HasPrefix(name, piDir)) {
				procs[cur].File = name
			}
		}
	}
}

func isSessionFile(name, home string) bool {
	return strings.HasSuffix(name, ".jsonl") &&
		(strings.HasPrefix(name, filepath.Join(home, ".codex", "sessions")+"/") ||
			strings.HasPrefix(name, filepath.Join(home, ".pi", "agent", "sessions")+"/"))
}

func fillFromProc(procs []Proc, idx map[int]int, home string) {
	for pid, i := range idx {
		base := filepath.Join("/proc", strconv.Itoa(pid))
		if cwd, err := os.Readlink(filepath.Join(base, "cwd")); err == nil {
			procs[i].Cwd = cwd
		}
		fds, _ := os.ReadDir(filepath.Join(base, "fd"))
		for _, fd := range fds {
			if target, err := os.Readlink(filepath.Join(base, "fd", fd.Name())); err == nil && isSessionFile(target, home) {
				procs[i].File = target
			}
		}
	}
}
