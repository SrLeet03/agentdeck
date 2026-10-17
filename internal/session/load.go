package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"
)

var (
	homeOnce sync.Once
	home     string
)

func homeDir() string {
	homeOnce.Do(func() { home, _ = os.UserHomeDir() })
	return home
}

// DataDir is where agentdeck keeps its own state and cache.
func DataDir() string {
	d := filepath.Join(homeDir(), ".agentdeck")
	os.MkdirAll(d, 0o755)
	return d
}

const cacheVersion = 4

type cacheEntry struct {
	ModTime int64    `json:"m"`
	Size    int64    `json:"s"`
	Session *Session `json:"x"`
}

type cacheFile struct {
	Version int                    `json:"v"`
	Entries map[string]*cacheEntry `json:"e"`
}

// Loader parses session files incrementally, re-reading only files whose size or mtime changed.
type Loader struct {
	mu    sync.Mutex
	cache cacheFile
	dirty bool
}

func NewLoader() *Loader {
	l := &Loader{cache: cacheFile{Version: cacheVersion, Entries: map[string]*cacheEntry{}}}
	if b, err := os.ReadFile(filepath.Join(DataDir(), "cache.json")); err == nil {
		var c cacheFile
		if json.Unmarshal(b, &c) == nil && c.Version == cacheVersion && c.Entries != nil {
			l.cache = c
		}
	}
	return l
}

func (l *Loader) Save() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.dirty {
		return
	}
	b, err := json.Marshal(l.cache)
	if err != nil {
		return
	}
	tmp := filepath.Join(DataDir(), "cache.json.tmp")
	if os.WriteFile(tmp, b, 0o644) == nil {
		os.Rename(tmp, filepath.Join(DataDir(), "cache.json"))
		l.dirty = false
	}
}

type job struct {
	agent Agent
	path  string
	mod   int64
	size  int64
}

// Load returns every non-empty session, newest first.
func (l *Loader) Load() []*Session {
	var jobs []job
	add := func(a Agent, files []string) {
		for _, f := range files {
			st, err := os.Stat(f)
			if err != nil {
				continue
			}
			jobs = append(jobs, job{a, f, st.ModTime().UnixNano(), st.Size()})
		}
	}
	add(Claude, claudeFiles())
	add(Codex, codexFiles())
	add(Pi, piFiles())

	titles := codexTitles()
	out := make([]*Session, len(jobs))
	var todo []int
	l.mu.Lock()
	present := map[string]bool{}
	for i, j := range jobs {
		present[j.path] = true
		if e := l.cache.Entries[j.path]; e != nil && e.ModTime == j.mod && e.Size == j.size && e.Session != nil {
			out[i] = e.Session
		} else {
			todo = append(todo, i)
		}
	}
	for p := range l.cache.Entries {
		if !present[p] {
			delete(l.cache.Entries, p)
			l.dirty = true
		}
	}
	l.mu.Unlock()

	if len(todo) > 0 {
		var wg sync.WaitGroup
		ch := make(chan int)
		for w := 0; w < runtime.NumCPU(); w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range ch {
					j := jobs[i]
					var s *Session
					switch j.agent {
					case Claude:
						s, _ = parseClaude(j.path)
					case Codex:
						s, _ = parseCodex(j.path, titles)
					case Pi:
						s, _ = parsePi(j.path)
					}
					out[i] = s
					l.mu.Lock()
					l.cache.Entries[j.path] = &cacheEntry{j.mod, j.size, s}
					l.dirty = true
					l.mu.Unlock()
				}
			}()
		}
		for _, i := range todo {
			ch <- i
		}
		close(ch)
		wg.Wait()
	}

	res := make([]*Session, 0, len(out))
	for _, s := range out {
		if s == nil || s.ID == "" || (s.UserMsgs == 0 && s.AgentMsgs == 0) {
			continue
		}
		if s.Agent == Codex {
			if t := titles[s.ID]; t != "" {
				s.Title = t
			}
		}
		res = append(res, s)
	}
	sort.SliceStable(res, func(a, b int) bool { return res[a].Updated.After(res[b].Updated) })
	return res
}

// Messages returns the tail of the conversation for the preview pane; truncated reports older history was skipped.
func Messages(s *Session) ([]Message, bool) {
	switch s.Agent {
	case Claude:
		return claudeMessages(s.Path)
	case Codex:
		return codexMessages(s.Path)
	case Pi:
		return piMessages(s.Path)
	}
	return nil, false
}

// ModTime is used by live detection to judge whether an agent is actively writing.
func ModTime(path string) time.Time {
	st, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}
