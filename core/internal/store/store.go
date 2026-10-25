// Package store keeps the core's durable state as small JSON files under one data
// directory. Writes are atomic (temp file, fsync, rename, fsync dir) and only happen
// when something changed, to spare the router's NAND flash.
package store

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const Retention = 30 // days of app activity kept (D6)

var (
	ErrInvalidID = errors.New("invalid id")
	validID      = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

type Store struct {
	dir    string
	mu     sync.Mutex
	pruned string // day of the last automatic prune
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "usage"), 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// writeJSON replaces path atomically, so a power cut leaves the old or the new file.
func writeJSON(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func readJSON(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	const maxDocument = 1 << 20
	data, err := io.ReadAll(io.LimitReader(f, maxDocument+1))
	if err != nil {
		return err
	}
	if len(data) > maxDocument {
		return errors.New("document exceeds 1 MiB")
	}
	return json.Unmarshal(data, v)
}

// ReadDocument reads a top-level JSON document (e.g. "devices.json"); found is false
// when it does not exist yet. Callers own the schema and its version field.
func (s *Store) ReadDocument(name string, v any) (found bool, err error) {
	if filepath.Base(name) != name || !strings.HasSuffix(name, ".json") {
		return false, ErrInvalidID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err = readJSON(filepath.Join(s.dir, name), v)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// WriteDocument atomically replaces a top-level JSON document.
func (s *Store) WriteDocument(name string, v any) error {
	if filepath.Base(name) != name || !strings.HasSuffix(name, ".json") {
		return ErrInvalidID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSON(filepath.Join(s.dir, name), v)
}

// Report is one phone's cumulative usage for one router-local day (protocol §5).
// Seconds; Total is the screen-on union (PiP counts once), not the sum of Apps.
type Report struct {
	Device     string            `json:"device"` // storage key: device id + key fingerprint
	Name       string            `json:"name,omitempty"`
	Day        string            `json:"day"`
	Seq        int64             `json:"seq"`
	Total      int               `json:"total"`
	Apps       map[string]int    `json:"apps"`
	Labels     map[string]string `json:"labels,omitempty"`
	Grants     map[string]int    `json:"grants,omitempty"`
	Incomplete bool              `json:"incomplete,omitempty"`
	Received   int64             `json:"received"`
}

const (
	maxDaySeconds   = 25 * 3600 // a DST day can have 25 hours
	maxReportApps   = 256
	maxReportGrants = 32
	maxDayDevices   = 16
)

var (
	ErrRegressed = errors.New("usage_regressed")
	ErrFull      = errors.New("usage_full")
	validPkg     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)
	validDevKey  = regexp.MustCompile(`^[A-Za-z0-9-]{1,80}@[0-9a-f]{16}$`)
	validGrant   = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

func (r Report) valid() bool {
	if !validDevKey.MatchString(r.Device) || r.Seq < 1 || r.Total < 0 || r.Total > maxDaySeconds ||
		len(r.Apps) > maxReportApps || len(r.Labels) > maxReportApps || len(r.Grants) > maxReportGrants ||
		len([]rune(r.Name)) > 64 {
		return false
	}
	for pkg, secs := range r.Apps {
		if !validPkg.MatchString(pkg) || len(pkg) > 255 || secs < 0 || secs > maxDaySeconds {
			return false
		}
	}
	for pkg, label := range r.Labels {
		if _, ok := r.Apps[pkg]; !ok || len([]rune(label)) > 64 {
			return false
		}
	}
	for id, secs := range r.Grants {
		if !validGrant.MatchString(id) || secs < 0 || secs > 240*60 {
			return false
		}
	}
	return true
}

func (s *Store) usagePath(profile, day string) (string, error) {
	if !validID.MatchString(profile) {
		return "", ErrInvalidID
	}
	if t, err := time.Parse(time.DateOnly, day); err != nil || t.Format(time.DateOnly) != day {
		return "", ErrInvalidID
	}
	return filepath.Join(s.dir, "usage", profile, day+".json"), nil
}

// ApplyUsage records a cumulative report. Only a higher seq can change the stored
// report, and counters never go backwards within a day: retries, out-of-order
// duplicates and a replayed older report cannot lower or double count usage.
// Identical values are not rewritten (flash wear).
func (s *Store) ApplyUsage(profile string, r Report) (changed bool, err error) {
	if !r.valid() {
		return false, ErrInvalidID
	}
	path, err := s.usagePath(profile, r.Day)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	day := map[string]Report{}
	if err := readJSON(path, &day); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	prev, ok := day[r.Device]
	if ok {
		if r.Seq <= prev.Seq {
			return false, nil
		}
		if r.Total < prev.Total {
			return false, ErrRegressed
		}
		for pkg, secs := range prev.Apps {
			if r.Apps[pkg] < secs {
				return false, ErrRegressed
			}
		}
		for id, secs := range prev.Grants {
			if r.Grants[id] < secs {
				return false, ErrRegressed
			}
		}
		if sameUsage(prev, r) {
			return false, nil
		}
	} else if len(day) >= maxDayDevices {
		return false, ErrFull
	}
	day[r.Device] = r
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	if err := writeJSON(path, day); err != nil {
		return false, err
	}
	if today := time.Now(); s.pruned != today.Format(time.DateOnly) {
		if err := s.pruneLocked(today); err == nil {
			s.pruned = today.Format(time.DateOnly)
		}
	}
	return true, nil
}

func sameUsage(a, b Report) bool {
	a.Seq, b.Seq, a.Received, b.Received = 0, 0, 0, 0
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// UsageDay returns every phone's last report for one day, keyed by storage key.
func (s *Store) UsageDay(profile, day string) (map[string]Report, error) {
	path, err := s.usagePath(profile, day)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	reports := map[string]Report{}
	if err := readJSON(path, &reports); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return reports, nil
}

// Prune deletes usage older than the retention window ending at today.
func (s *Store) Prune(today time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pruneLocked(today)
}

func (s *Store) pruneLocked(today time.Time) error {
	cutoff := today.AddDate(0, 0, -(Retention - 1)).Format(time.DateOnly)
	files, err := filepath.Glob(filepath.Join(s.dir, "usage", "*", "*.json"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, f := range files {
		if day := filepath.Base(f[:len(f)-len(".json")]); day < cutoff {
			if err := os.Remove(f); err != nil {
				return err
			}
		}
	}
	return nil
}
