package state

import (
	"reflect"
	"testing"
)

func TestParseTags(t *testing.T) {
	got := ParseTags(" Voice, #demo  voice,urgent ")
	want := []string{"demo", "urgent", "voice"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestStoreDropsEmptyEntries(t *testing.T) {
	s := Open(t.TempDir())
	s.Update("claude:x", func(m *Meta) { m.Pinned = true })
	s.Update("claude:x", func(m *Meta) { m.Pinned = false })
	if _, ok := s.Sessions["claude:x"]; ok {
		t.Fatal("empty meta should be removed")
	}
}

func TestGroups(t *testing.T) {
	s := Open(t.TempDir())
	name, _ := s.AddToGroup([]string{"claude:a", "codex:b"}, "Launch Week")
	if again, _ := s.AddToGroup([]string{"pi:c"}, "launch week"); again != name {
		t.Fatalf("case-insensitive match failed: %q vs %q", again, name)
	}
	if len(s.Groups) != 1 || !s.Get("pi:c").InGroup("Launch Week") {
		t.Fatalf("groups = %v", s.Groups)
	}
	if err := s.RenameGroup("Launch Week", "launch"); err != nil || !s.Get("codex:b").InGroup("launch") {
		t.Fatalf("rename failed: %v", err)
	}
	s.RemoveFromGroup([]string{"claude:a"}, "launch")
	if s.Get("claude:a").InGroup("launch") {
		t.Fatal("remove failed")
	}
	s.DeleteGroup("launch")
	if len(s.Groups) != 0 || len(s.Sessions) != 0 {
		t.Fatalf("delete left %v %v", s.Groups, s.Sessions)
	}
}
