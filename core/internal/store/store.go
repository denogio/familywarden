// Package store keeps the core's durable state as small JSON files under one data
// directory. Writes are atomic (temp file, fsync, rename, fsync dir) and only happen
// when something changed, to spare the router's NAND flash.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/denogio/familywarden/core/internal/protocol"
)

const Retention = 30 // days of app activity kept (D6)

var (
	ErrRequestOpen = errors.New("request_open")
	ErrInvalidID   = errors.New("invalid id")
	validID        = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

type Store struct {
	dir string
	mu  sync.Mutex
}

func Open(dir string) (*Store, error) {
	for _, sub := range []string{"usage", "requests"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return nil, err
		}
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
	data, err := os.ReadFile(path)
	if err != nil {
		return err
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

// OpenRequest stores a child's request; at most one is open per profile (D7).
// An expired request no longer counts as open.
func (s *Store) OpenRequest(r protocol.Request, now time.Time) error {
	if !validID.MatchString(r.Profile) {
		return ErrInvalidID
	}
	path := filepath.Join(s.dir, "requests", r.Profile+".json")
	s.mu.Lock()
	defer s.mu.Unlock()
	var cur protocol.Request
	if err := readJSON(path, &cur); err == nil && now.Sub(time.Unix(cur.Created, 0)) < protocol.RequestTTL {
		if cur.RequestID == r.RequestID {
			return nil // a retry of the same request
		}
		return ErrRequestOpen
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeJSON(path, r)
}

// OpenRequestFor returns the profile's open request, or nil if none or expired.
func (s *Store) OpenRequestFor(profile string, now time.Time) (*protocol.Request, error) {
	if !validID.MatchString(profile) {
		return nil, ErrInvalidID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var cur protocol.Request
	err := readJSON(filepath.Join(s.dir, "requests", profile+".json"), &cur)
	if errors.Is(err, os.ErrNotExist) || (err == nil && now.Sub(time.Unix(cur.Created, 0)) >= protocol.RequestTTL) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cur, nil
}

// CloseRequest removes the profile's request once answered (approved or denied).
func (s *Store) CloseRequest(profile, requestID string) error {
	if !validID.MatchString(profile) {
		return ErrInvalidID
	}
	path := filepath.Join(s.dir, "requests", profile+".json")
	s.mu.Lock()
	defer s.mu.Unlock()
	var cur protocol.Request
	if err := readJSON(path, &cur); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if cur.RequestID != requestID {
		return nil
	}
	return os.Remove(path)
}
