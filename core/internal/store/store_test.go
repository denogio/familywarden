package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func open(t *testing.T) (*Store, string) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s, dir
}

func TestUsageIsCumulativeIdempotentAndShared(t *testing.T) {
	s, _ := open(t)
	apply := func(dev string, seq, secs int) bool {
		changed, err := s.ApplyUsage("alex", Report{Device: dev, Day: "2026-10-21", Seq: seq, Apps: map[string]int{"com.roblox.client": secs}})
		if err != nil {
			t.Fatal(err)
		}
		return changed
	}
	if !apply("phone", 1, 600) || !apply("phone", 2, 900) {
		t.Fatal("new reports not applied")
	}
	if apply("phone", 2, 900) || apply("phone", 1, 600) {
		t.Fatal("retry or out-of-order report applied again")
	}
	apply("tablet", 1, 300)
	got, err := s.Usage("alex", "2026-10-21")
	if err != nil || got["com.roblox.client"] != 1200 {
		t.Fatalf("shared usage = %v, %v; want 900+300", got, err)
	}
	// The same state survives a restart (a new Store on the same directory).
	again, _ := Open(s.dir)
	if got, _ := again.Usage("alex", "2026-10-21"); got["com.roblox.client"] != 1200 {
		t.Fatal("usage not durable")
	}
}

func TestRejectsPathTricks(t *testing.T) {
	s, _ := open(t)
	for _, bad := range []struct{ profile, device, day string }{
		{"../etc", "phone", "2026-10-21"},
		{"alex", "../phone", "2026-10-21"},
		{"alex", "phone", "../../x"},
		{"alex", "phone", "2026-13-40"},
	} {
		if _, err := s.ApplyUsage(bad.profile, Report{Device: bad.device, Day: bad.day, Seq: 1}); err != ErrInvalidID {
			t.Errorf("%+v: got %v", bad, err)
		}
	}
}

func TestPruneKeepsThirtyDays(t *testing.T) {
	s, dir := open(t)
	today := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	for _, back := range []int{0, 29, 30, 45} {
		day := today.AddDate(0, 0, -back).Format(time.DateOnly)
		if _, err := s.ApplyUsage("alex", Report{Device: "phone", Day: day, Seq: 1, Apps: map[string]int{"a": 1}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Prune(today); err != nil {
		t.Fatal(err)
	}
	left, _ := filepath.Glob(filepath.Join(dir, "usage", "alex", "*.json"))
	if len(left) != 2 {
		t.Fatalf("kept %v, want today and 29 days ago", left)
	}
}

func TestAtomicWriteLeavesNoTempFiles(t *testing.T) {
	s, dir := open(t)
	if _, err := s.ApplyUsage("alex", Report{Device: "phone", Day: "2026-10-21", Seq: 1}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "usage", "alex"))
	if len(entries) != 1 || entries[0].Name() != "2026-10-21.json" {
		t.Fatalf("unexpected files: %v", entries)
	}
}
