package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func codexFiles() []string {
	var files []string
	root := filepath.Join(homeDir(), ".codex", "sessions")
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".jsonl") {
			files = append(files, p)
		}
		return nil
	})
	return files
}

// codexTitles reads Codex's thread-name index (last write wins).
func codexTitles() map[string]string {
	m := map[string]string{}
	eachLine(filepath.Join(homeDir(), ".codex", "session_index.jsonl"), func(b []byte) {
		var e struct {
			ID   string `json:"id"`
			Name string `json:"thread_name"`
		}
		if json.Unmarshal(b, &e) == nil && e.ID != "" && e.Name != "" {
			m[e.ID] = e.Name
		}
	})
	return m
}

type codexLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexUsage struct {
	In     int64 `json:"input_tokens"`
	Cached int64 `json:"cached_input_tokens"`
	CW     int64 `json:"cache_write_input_tokens"`
	Out    int64 `json:"output_tokens"`
}

func (u codexUsage) tokens() Tokens {
	in := u.In - u.Cached - u.CW
	if in < 0 {
		in = 0
	}
	return Tokens{In: in, Out: u.Out, CacheRead: u.Cached, CacheWrite: u.CW}
}

type codexPayload struct {
	Type          string `json:"type"`
	ID            string `json:"id"`
	Cwd           string `json:"cwd"`
	Timestamp     string `json:"timestamp"`
	Originator    string `json:"originator"`
	ThreadSource  string `json:"thread_source"`
	ParentThread  string `json:"parent_thread_id"`
	AgentNickname string `json:"agent_nickname"`
	Model         string `json:"model"`
	Git           *struct {
		Branch string `json:"branch"`
	} `json:"git"`
	Role           string          `json:"role"`
	Content        json.RawMessage `json:"content"`
	Name           string          `json:"name"`
	Arguments      string          `json:"arguments"`
	Input          string          `json:"input"`
	ResponseID     string          `json:"response_id"`
	Usage          *codexUsage     `json:"usage"`
	ThreadSettings *struct {
		Model string `json:"model"`
	} `json:"thread_settings"`
	Info *struct {
		Total *codexUsage `json:"total_token_usage"`
		Last  *codexUsage `json:"last_token_usage"`
	} `json:"info"`
}

func parseCodex(path string, titles map[string]string) (*Session, error) {
	s := &Session{Agent: Codex, Path: path, Daily: map[string]int64{}}
	seenResp := map[string]bool{}
	haveRecords := false
	var fallback Tokens
	var lastTotal int64
	fallbackDaily := map[string]int64{}
	err := eachLine(path, func(b []byte) {
		switch lineType(b) {
		case "world_state", "compacted":
			// Huge snapshots; only the timestamp would matter and neighbouring lines carry it.
			return
		}
		var l codexLine
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
		// Skip the bulky snapshots before decoding the payload.
		if l.Type == "world_state" || l.Type == "compacted" {
			return
		}
		var p codexPayload
		if json.Unmarshal(l.Payload, &p) != nil {
			return
		}
		switch l.Type {
		case "session_meta":
			if s.ID == "" {
				s.ID = p.ID
				s.Cwd = p.Cwd
				s.Source = p.Originator
				if t := parseTime(p.Timestamp); !t.IsZero() {
					s.Created = t
				}
				if p.ThreadSource == "subagent" {
					s.Subagent = true
					s.ParentID = p.ParentThread
					if p.AgentNickname != "" {
						s.Title = "↳ " + p.AgentNickname
					}
				}
				if p.Git != nil {
					s.Branch = p.Git.Branch
				}
			}
		case "turn_context":
			if p.Model != "" {
				s.Model = p.Model
			}
		case "event_msg":
			switch p.Type {
			case "thread_settings_applied":
				if p.ThreadSettings != nil && p.ThreadSettings.Model != "" {
					s.Model = p.ThreadSettings.Model
				}
			case "token_count":
				if p.Info != nil && p.Info.Total != nil && p.Info.Last != nil {
					tot := p.Info.Total.In + p.Info.Total.Out
					if tot != lastTotal {
						lastTotal = tot
						fallback = p.Info.Total.tokens()
						addDaily(fallbackDaily, ts, p.Info.Last.tokens().Total())
					}
				}
			}
		case "token_usage_record":
			if p.Usage != nil && !seenResp[p.ResponseID] {
				seenResp[p.ResponseID] = true
				haveRecords = true
				t := p.Usage.tokens()
				s.Tokens.Add(t)
				addDaily(s.Daily, ts, t.Total())
			}
		case "response_item":
			if p.Type != "message" {
				return
			}
			text, _ := textOf(p.Content)
			switch p.Role {
			case "user":
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
			}
		}
	})
	if !haveRecords {
		s.Tokens = fallback
		s.Daily = fallbackDaily
	}
	if s.ID == "" {
		// rollout-<ts>-<uuid>.jsonl
		base := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		if len(base) >= 36 {
			s.ID = base[len(base)-36:]
		}
	}
	if t := titles[s.ID]; t != "" {
		s.Title = t
	}
	return s, err
}

func codexMessages(path string) ([]Message, bool) {
	var out []Message
	truncated, _ := eachLineTail(path, PreviewTailBytes, func(b []byte) {
		if lineType(b) != "response_item" {
			return
		}
		var l codexLine
		if json.Unmarshal(b, &l) != nil || l.Type != "response_item" {
			return
		}
		var p codexPayload
		if json.Unmarshal(l.Payload, &p) != nil {
			return
		}
		ts := parseTime(l.Timestamp)
		switch p.Type {
		case "message":
			text, _ := textOf(p.Content)
			if p.Role == "user" && !isInjected(text) {
				out = append(out, Message{"user", text, ts})
			} else if p.Role == "assistant" && strings.TrimSpace(text) != "" {
				out = append(out, Message{"assistant", text, ts})
			}
		case "function_call":
			out = append(out, Message{"tool", p.Name + " " + toolSummary(json.RawMessage(p.Arguments)), ts})
		case "custom_tool_call", "local_shell_call":
			out = append(out, Message{"tool", p.Name + " " + firstLine(p.Input), ts})
		}
	})
	return out, truncated
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " …"
	}
	return s
}
