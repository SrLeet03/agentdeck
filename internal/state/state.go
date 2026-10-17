// Package state stores agentdeck's own per-session metadata. It never modifies agent files.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Meta struct {
	Name     string   `json:"name,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Pinned   bool     `json:"pinned,omitempty"`
	Archived bool     `json:"archived,omitempty"`
	Groups   []string `json:"groups,omitempty"`
}

func (m Meta) empty() bool {
	return m.Name == "" && len(m.Tags) == 0 && !m.Pinned && !m.Archived && len(m.Groups) == 0
}

// InGroup reports whether the session belongs to group g.
func (m Meta) InGroup(g string) bool {
	for _, x := range m.Groups {
		if x == g {
			return true
		}
	}
	return false
}

type Store struct {
	path     string
	Sessions map[string]Meta `json:"sessions"`
	// Groups lists user-made session groups in tab order (a group may be empty).
	Groups []string `json:"groups,omitempty"`
}

func Open(dir string) *Store {
	s := &Store{path: filepath.Join(dir, "state.json"), Sessions: map[string]Meta{}}
	if b, err := os.ReadFile(s.path); err == nil {
		json.Unmarshal(b, s)
		if s.Sessions == nil {
			s.Sessions = map[string]Meta{}
		}
	}
	return s
}

func (s *Store) Get(key string) Meta { return s.Sessions[key] }

func (s *Store) Update(key string, fn func(*Meta)) error {
	m := s.Sessions[key]
	fn(&m)
	if m.empty() {
		delete(s.Sessions, key)
	} else {
		s.Sessions[key] = m
	}
	return s.save()
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// ParseTags turns "a, b  c" into a sorted, de-duplicated tag list.
func ParseTags(in string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range strings.FieldsFunc(in, func(r rune) bool { return r == ',' || r == ' ' }) {
		t = strings.ToLower(strings.TrimPrefix(t, "#"))
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// ---- groups -------------------------------------------------------------------

// FindGroup returns the existing group name matching name case-insensitively.
func (s *Store) FindGroup(name string) (string, bool) {
	for _, g := range s.Groups {
		if strings.EqualFold(g, name) {
			return g, true
		}
	}
	return "", false
}

// AddToGroup puts sessions into group name, creating it if needed; it returns the canonical group name.
func (s *Store) AddToGroup(keys []string, name string) (string, error) {
	name = strings.TrimSpace(name)
	if g, ok := s.FindGroup(name); ok {
		name = g
	} else {
		s.Groups = append(s.Groups, name)
	}
	for _, k := range keys {
		m := s.Sessions[k]
		if !m.InGroup(name) {
			m.Groups = append(m.Groups, name)
		}
		s.Sessions[k] = m
	}
	return name, s.save()
}

// RemoveFromGroup takes sessions out of a group; the group itself stays.
func (s *Store) RemoveFromGroup(keys []string, name string) error {
	for _, k := range keys {
		s.Update(k, func(m *Meta) { m.Groups = without(m.Groups, name) })
	}
	return s.save()
}

// RenameGroup renames a group everywhere.
func (s *Store) RenameGroup(old, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || name == old {
		return nil
	}
	if g, ok := s.FindGroup(name); ok && g != old {
		return fmt.Errorf("a group called %q already exists", g)
	}
	for i, g := range s.Groups {
		if g == old {
			s.Groups[i] = name
		}
	}
	for k, m := range s.Sessions {
		for i, g := range m.Groups {
			if g == old {
				m.Groups[i] = name
			}
		}
		s.Sessions[k] = m
	}
	return s.save()
}

// DeleteGroup removes a group and every membership in it; sessions are untouched.
func (s *Store) DeleteGroup(name string) error {
	s.Groups = without(s.Groups, name)
	for k, m := range s.Sessions {
		m.Groups = without(m.Groups, name)
		if m.empty() {
			delete(s.Sessions, k)
		} else {
			s.Sessions[k] = m
		}
	}
	return s.save()
}

func without(list []string, x string) []string {
	var out []string
	for _, v := range list {
		if v != x {
			out = append(out, v)
		}
	}
	return out
}
