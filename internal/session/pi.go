package session

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

func piFiles() []string {
	files, _ := filepath.Glob(filepath.Join(homeDir(), ".pi", "agent", "sessions", "*", "*.jsonl"))
	return files
}

type piLine struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
	ModelID   string `json:"modelId"`
	Name      string `json:"name"`
	Message   *struct {
		Role     string          `json:"role"`
		Content  json.RawMessage `json:"content"`
		Model    string          `json:"model"`
		ToolName string          `json:"toolName"`
		Usage    *struct {
			In  int64 `json:"input"`
			Out int64 `json:"output"`
			CR  int64 `json:"cacheRead"`
			CW  int64 `json:"cacheWrite"`
		} `json:"usage"`
	} `json:"message"`
}

func parsePi(path string) (*Session, error) {
	s := &Session{Agent: Pi, Path: path, Daily: map[string]int64{}, Source: "pi"}
	err := eachLine(path, func(b []byte) {
		var l piLine
		if json.Unmarshal(b, &l) != nil {
			return
		}
		ts := parseTime(l.Timestamp)
		if !ts.IsZero() {
			if s.Created.IsZero() {
				s.Created = ts
			}
			if ts.After(s.Updated) {
				s.Updated = ts
			}
		}
		switch l.Type {
		case "session":
			s.ID, s.Cwd = l.ID, l.Cwd
		case "model_change":
			s.Model = l.ModelID
		case "session_info":
			if l.Name != "" {
				s.Title = l.Name
			}
		case "message":
			m := l.Message
			if m == nil {
				return
			}
			switch m.Role {
			case "user":
				text, _ := textOf(m.Content)
				if isInjected(text) {
					return
				}
				s.UserMsgs++
				if s.FirstPrompt == "" {
					s.FirstPrompt = text
				}
				s.LastPrompt = text
			case "assistant":
				s.AgentMsgs++
				if m.Model != "" {
					s.Model = m.Model
				}
				if u := m.Usage; u != nil {
					t := Tokens{In: u.In, Out: u.Out, CacheRead: u.CR, CacheWrite: u.CW}
					s.Tokens.Add(t)
					addDaily(s.Daily, ts, t.Total())
				}
			}
		}
	})
	return s, err
}

func piMessages(path string) ([]Message, bool) {
	var out []Message
	truncated, _ := eachLineTail(path, PreviewTailBytes, func(b []byte) {
		var l piLine
		if json.Unmarshal(b, &l) != nil || l.Type != "message" || l.Message == nil {
			return
		}
		ts := parseTime(l.Timestamp)
		m := l.Message
		switch m.Role {
		case "user":
			if text, _ := textOf(m.Content); !isInjected(text) {
				out = append(out, Message{"user", text, ts})
			}
		case "assistant":
			var blocks []contentBlock
			if json.Unmarshal(m.Content, &blocks) != nil {
				return
			}
			for _, bl := range blocks {
				switch bl.Type {
				case "text":
					if strings.TrimSpace(bl.Text) != "" {
						out = append(out, Message{"assistant", bl.Text, ts})
					}
				case "toolCall":
					out = append(out, Message{"tool", bl.Name + " " + toolSummary(bl.Arguments), ts})
				}
			}
		}
	})
	return out, truncated
}
