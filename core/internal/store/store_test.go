package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

const phoneKey = "fw-alex@0123456789abcdef"

func report(dev string, seq int64, total int, apps map[string]int) Report {
	return Report{Device: dev, Day: "2026-10-21", Seq: seq, Total: total, Apps: apps}
}

func TestUsageIsCumulativeMonotonicAndPerDevice(t *testing.T) {
	s, _ := open(t)
	apply := func(r Report) (bool, error) { return s.ApplyUsage("alex", r) }
	if ok, err := apply(report(phoneKey, 1, 600, map[string]int{"com.example.video": 600})); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if ok, err := apply(report(phoneKey, 2, 900, map[string]int{"com.example.video": 900})); !ok || err != nil {
		t.Fatal(ok, err)
	}
	// Retry, out-of-order, identical values: no change, no error.
	for _, r := range []Report{report(phoneKey, 2, 900, map[string]int{"com.example.video": 900}),
		report(phoneKey, 1, 600, map[string]int{"com.example.video": 600}),
		report(phoneKey, 3, 900, map[string]int{"com.example.video": 900})} {
		if ok, err := apply(r); ok || err != nil {
			t.Fatalf("%+v: %v %v", r, ok, err)
		}
	}
	// A higher seq may never lower a counter.
	for _, r := range []Report{report(phoneKey, 9, 800, map[string]int{"com.example.video": 900}),
		report(phoneKey, 9, 900, map[string]int{"com.example.video": 899}),
		report(phoneKey, 9, 900, map[string]int{})} {
		if _, err := apply(r); err != ErrRegressed {
			t.Fatalf("%+v: %v", r, err)
		}
	}
	apply(report("fw-tablet@fedcba9876543210", 1, 300, map[string]int{"com.example.video": 300}))
	again, _ := Open(s.dir)
	day, err := again.UsageDay("alex", "2026-10-21")
	if err != nil || len(day) != 2 || day[phoneKey].Total != 900 || day["fw-tablet@fedcba9876543210"].Total != 300 {
		t.Fatalf("not durable per device: %v %v", day, err)
	}
}

func TestUsageRejectsUnboundedOrMalformedReports(t *testing.T) {
	s, _ := open(t)
	big := map[string]int{}
	for i := 0; i <= maxReportApps; i++ {
		big[fmt.Sprintf("com.example.a%d", i)] = 1
	}
	for name, r := range map[string]Report{
		"device without key": report("fw-alex", 1, 1, nil),
		"path trick":         report("../x@0123456789abcdef", 1, 1, nil),
		"zero seq":           report(phoneKey, 0, 1, nil),
		"negative total":     report(phoneKey, 1, -1, nil),
		"longer than a day":  report(phoneKey, 1, maxDaySeconds+1, nil),
		"bad package":        report(phoneKey, 1, 1, map[string]int{"../x": 1}),
		"too many apps":      report(phoneKey, 1, 1, big),
		"label for no app":   {Device: phoneKey, Day: "2026-10-21", Seq: 1, Labels: map[string]string{"com.example.x": "X"}},
		"oversized grant":    {Device: phoneKey, Day: "2026-10-21", Seq: 1, Grants: map[string]int{strings.Repeat("a", 32): 240*60 + 1}},
		"bad date":           {Device: phoneKey, Day: "2026-13-40", Seq: 1},
		"non-canonical date": {Device: phoneKey, Day: "2026-1-2", Seq: 1},
	} {
		if _, err := s.ApplyUsage("alex", r); err != ErrInvalidID {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := s.ApplyUsage("../etc", report(phoneKey, 1, 1, nil)); err != ErrInvalidID {
		t.Error("profile path trick accepted")
	}
	for i := 0; i < maxDayDevices; i++ {
		if _, err := s.ApplyUsage("alex", report(fmt.Sprintf("fw-%d@0123456789abcdef", i), 1, 1, nil)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.ApplyUsage("alex", report("fw-extra@0123456789abcdef", 1, 1, nil)); err != ErrFull {
		t.Fatalf("device bound: %v", err)
	}
}

func TestPruneKeepsThirtyDays(t *testing.T) {
	s, dir := open(t)
	today := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	for _, back := range []int{0, 29, 30, 45} {
		r := report(phoneKey, 1, 1, nil)
		r.Day = today.AddDate(0, 0, -back).Format(time.DateOnly)
		if _, err := s.ApplyUsage("alex", r); err != nil {
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
	witnesses, _ := filepath.Glob(filepath.Join(dir, "usage", "alex", "*.json"+PresenceSuffix))
	if len(witnesses) != 2 {
		t.Fatal("retention left orphaned presence witnesses", witnesses)
	}
	if err := s.CheckPresence(); err != nil {
		t.Fatal("intentional retention deletion became data loss", err)
	}
}

func TestAtomicWriteLeavesNoTempFiles(t *testing.T) {
	s, dir := open(t)
	if _, err := s.ApplyUsage("alex", report(phoneKey, 1, 0, nil)); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "usage", "alex"))
	if len(entries) != 2 || entries[0].Name() != "2026-10-21.json" || entries[1].Name() != "2026-10-21.json"+PresenceSuffix {
		t.Fatalf("unexpected files: %v", entries)
	}
}
