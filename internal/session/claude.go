package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func eachLine(path string, fn func(line []byte)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 512<<20)
	for sc.Scan() {
		if b := sc.Bytes(); len(b) > 0 {
			fn(b)
		}
	}
	return sc.Err()
}

// eachLineTail is eachLine over roughly the last maxBytes of the file (whole lines only).
func eachLineTail(path string, maxBytes int64, fn func(line []byte)) (truncated bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false, err
	}
	skipFirst := false
	if st.Size() > maxBytes {
		f.Seek(st.Size()-maxBytes, io.SeekStart)
		skipFirst, truncated = true, true
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 512<<20)
	for sc.Scan() {
		if skipFirst {
			skipFirst = false
			continue
		}
		if b := sc.Bytes(); len(b) > 0 {
			fn(b)
		}
	}
	return truncated, sc.Err()
}

// PreviewTailBytes bounds how much of a session file the preview reads.
const PreviewTailBytes = 8 << 20

// lineType cheaply peeks at the top-level "type" value near the start of a JSONL record.
func lineType(b []byte) string {
	head := b[:min(len(b), 256)]
	i := bytes.Index(head, []byte(`"type":"`))
	if i < 0 {
		return ""
	}
	rest := head[i+8:]
	j := bytes.IndexByte(rest, '"')
	if j < 0 {
		return ""
	}
	return string(rest[:j])
}

func claudeFiles() []string {
	files, _ := filepath.Glob(filepath.Join(homeDir(), ".claude", "projects", "*", "*.jsonl"))
	return files
}

type claudeLine struct {
	Type        string `json:"type"`
	SessionID   string `json:"sessionId"`
	Cwd         string `json:"cwd"`
	GitBranch   string `json:"gitBranch"`
	Timestamp   string `json:"timestamp"`
	IsSidechain bool   `json:"isSidechain"`
	Entrypoint  string `json:"entrypoint"`
	AiTitle     string `json:"aiTitle"`
	CustomTitle string `json:"customTitle"`
	AgentName   string `json:"agentName"`
	Summary     string `json:"summary"`
	IsMeta      bool   `json:"isMeta"`
	Message     *struct {
		ID      string          `json:"id"`
		Role    string          `json:"role"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   *struct {
			In  int64 `json:"input_tokens"`
			Out int64 `json:"output_tokens"`
			CR  int64 `json:"cache_read_input_tokens"`
			CW  int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	Arguments json.RawMessage `json:"arguments"`
	Content   json.RawMessage `json:"content"`
}

// textOf flattens a Claude/Pi style content field (string or block array) to visible text.
func textOf(raw json.RawMessage) (text string, toolOnly bool) {
	if len(raw) == 0 {
		return "", false
	}
	if raw[0] == '"' {
		var s string
		_ = json.Unmarshal(raw, &s)
		return s, false
	}
	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return "", false
	}
	var parts []string
	tools := 0
	for _, b := range blocks {
		switch b.Type {
		case "text", "input_text", "output_text":
			parts = append(parts, b.Text)
		case "tool_result", "tool_use", "toolCall", "thinking", "redacted_thinking":
			tools++
		}
	}
	return strings.Join(parts, "\n"), len(parts) == 0 && tools > 0
}

func parseClaude(path string) (*Session, error) {
	s := &Session{Agent: Claude, Path: path, Daily: map[string]int64{}}
	s.ID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	seenMsg := map[string]bool{}
	var aiTitle, customTitle, agentName, summary string
	err := eachLine(path, func(b []byte) {
		var l claudeLine
		if json.Unmarshal(b, &l) != nil {
			return
		}
		ts := parseTime(l.Timestamp)
		if !ts.IsZero() {
			if s.Created.IsZero() || ts.Before(s.Created) {
				s.Created = ts
			}
			if ts.After(s.Updated) {
				s.Updated = ts
			}
		}
		if l.Cwd != "" && s.Cwd == "" {
			s.Cwd = l.Cwd
		}
		if l.GitBranch != "" {
			s.Branch = l.GitBranch
		}
		if l.Entrypoint != "" && s.Source == "" {
			s.Source = l.Entrypoint
		}
		switch l.Type {
		case "ai-title":
			aiTitle = l.AiTitle
		case "custom-title":
			customTitle = l.CustomTitle
		case "agent-name":
			agentName = l.AgentName
		case "summary":
			summary = l.Summary
		case "user":
			if l.Message == nil || l.IsSidechain || l.IsMeta {
				return
			}
			text, toolOnly := textOf(l.Message.Content)
			if toolOnly {
				return
			}
			if c := commandName(text); c != "" {
				text = c
			} else if isInjected(text) {
				return
			}
			s.UserMsgs++
			if s.FirstPrompt == "" {
				s.FirstPrompt = text
			}
			s.LastPrompt = text
		case "assistant":
			if l.Message == nil || l.IsSidechain {
				return
			}
			if l.Message.Model != "" && l.Message.Model != "<synthetic>" {
				s.Model = l.Message.Model
			}
			// Claude streams one line per content block, repeating the message id and usage.
			if l.Message.ID != "" {
				if seenMsg[l.Message.ID] {
					return
				}
				seenMsg[l.Message.ID] = true
			}
			s.AgentMsgs++
			if u := l.Message.Usage; u != nil {
				t := Tokens{In: u.In, Out: u.Out, CacheRead: u.CR, CacheWrite: u.CW}
				s.Tokens.Add(t)
				addDaily(s.Daily, ts, t.Total())
			}
		}
	})
	switch {
	case customTitle != "":
		s.Title = customTitle
	case aiTitle != "":
		s.Title = aiTitle
	case agentName != "" && agentName != s.ID[:min(8, len(s.ID))]:
		s.Title = agentName
	case summary != "":
		s.Title = summary
	}
	addClaudeSubagents(s)
	if s.Updated.IsZero() {
		if st, e := os.Stat(path); e == nil {
			s.Updated = st.ModTime()
		}
	}
	return s, err
}

// Message is one rendered turn for the preview pane.
type Message struct {
	Role string // user | assistant | tool
	Text string
	At   time.Time
}

func claudeMessages(path string) ([]Message, bool) {
	var out []Message
	truncated, _ := eachLineTail(path, PreviewTailBytes, func(b []byte) {
		var l claudeLine
		if json.Unmarshal(b, &l) != nil || l.Message == nil || l.IsSidechain || l.IsMeta {
			return
		}
		ts := parseTime(l.Timestamp)
		switch l.Type {
		case "user":
			text, toolOnly := textOf(l.Message.Content)
			if toolOnly {
				return
			}
			if c := commandName(text); c != "" {
				text = c
			} else if isInjected(text) {
				return
			}
			out = append(out, Message{"user", text, ts})
		case "assistant":
			var blocks []contentBlock
			if json.Unmarshal(l.Message.Content, &blocks) != nil {
				return
			}
			for _, bl := range blocks {
				switch bl.Type {
				case "text":
					if strings.TrimSpace(bl.Text) != "" {
						out = append(out, Message{"assistant", bl.Text, ts})
					}
				case "tool_use":
					out = append(out, Message{"tool", bl.Name + " " + toolSummary(bl.Input), ts})
				}
			}
		}
	})
	return out, truncated
}

func toolSummary(raw json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	for _, k := range []string{"command", "cmd", "file_path", "path", "pattern", "description", "prompt", "url", "query"} {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// addClaudeSubagents folds token usage from <session>/subagents/**.jsonl into the parent session.
func addClaudeSubagents(s *Session) {
	dir := filepath.Join(strings.TrimSuffix(s.Path, ".jsonl"), "subagents")
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".jsonl") || !strings.HasPrefix(d.Name(), "agent-") {
			return nil
		}
		s.Subagents++
		seen := map[string]bool{}
		eachLine(p, func(b []byte) {
			var l claudeLine
			if json.Unmarshal(b, &l) != nil || l.Type != "assistant" || l.Message == nil || l.Message.Usage == nil {
				return
			}
			if l.Message.ID != "" {
				if seen[l.Message.ID] {
					return
				}
				seen[l.Message.ID] = true
			}
			u := l.Message.Usage
			t := Tokens{In: u.In, Out: u.Out, CacheRead: u.CR, CacheWrite: u.CW}
			s.Tokens.Add(t)
			addDaily(s.Daily, parseTime(l.Timestamp), t.Total())
		})
		return nil
	})
}
