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
	dir string
	mu  sync.Mutex
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

// Report is one phone's cumulative usage for one day (protocol §5).
type Report struct {
	Device string         `json:"device"`
	Day    string         `json:"day"`
	Seq    int            `json:"seq"`
	Apps   map[string]int `json:"apps"`
}

type deviceDay struct {
	Seq  int            `json:"seq"`
	Apps map[string]int `json:"apps"`
}

func (s *Store) usagePath(profile, day string) (string, error) {
	if !validID.MatchString(profile) {
		return "", ErrInvalidID
	}
	if _, err := time.Parse(time.DateOnly, day); err != nil {
		return "", ErrInvalidID
	}
	return filepath.Join(s.dir, "usage", profile, day+".json"), nil
}

// ApplyUsage records a cumulative report. Only a higher seq replaces the device's
// previous report, so retries and out-of-order duplicates never count twice.
func (s *Store) ApplyUsage(profile string, r Report) (changed bool, err error) {
	if !validID.MatchString(r.Device) {
		return false, ErrInvalidID
	}
	path, err := s.usagePath(profile, r.Day)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	day := map[string]deviceDay{}
	if err := readJSON(path, &day); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if prev, ok := day[r.Device]; ok && r.Seq <= prev.Seq {
		return false, nil
	}
	day[r.Device] = deviceDay{Seq: r.Seq, Apps: r.Apps}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	return true, writeJSON(path, day)
}

// Usage sums all of a child's devices for one day: the shared budget (D4).
func (s *Store) Usage(profile, day string) (map[string]int, error) {
	path, err := s.usagePath(profile, day)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	devices := map[string]deviceDay{}
	if err := readJSON(path, &devices); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	total := map[string]int{}
	for _, d := range devices {
		for pkg, secs := range d.Apps {
			if secs > 0 {
				total[pkg] += secs
			}
		}
	}
	return total, nil
}

// Prune deletes usage older than the retention window ending at today.
func (s *Store) Prune(today time.Time) error {
	cutoff := today.AddDate(0, 0, -(Retention - 1)).Format(time.DateOnly)
	s.mu.Lock()
	defer s.mu.Unlock()
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
