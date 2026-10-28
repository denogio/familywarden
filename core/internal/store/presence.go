package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PresenceSuffix witnesses a successfully written file, not its newest content.
// Native ownership routing also recognizes witnesses of its authority documents.
const PresenceSuffix = ".present"

type presence struct {
	Version int `json:"version"`
}

func checkPresence(path string) (bool, error) {
	var p presence
	err := readJSON(path+PresenceSuffix, &p)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if p.Version != 1 {
		return false, errors.New("unsupported document presence format")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return true, fmt.Errorf("previously written document unavailable: %s", filepath.Base(path))
	}
	return true, nil
}

// Callers hold Store.mu. A failed write can already have renamed its data: stop
// further operations in this instance instead of letting cached writers proceed.
func (s *Store) writeRecorded(path string, value any) error {
	if s.blocked != nil {
		return s.blocked
	}
	found, err := checkPresence(path)
	if err == nil {
		err = writeJSON(path, value)
	}
	if err == nil && !found {
		err = writeJSON(path+PresenceSuffix, presence{Version: 1})
	}
	if err != nil {
		// Do not unwrap ENOENT: an uncertain store is never first-use absence.
		s.blocked = fmt.Errorf("store write uncertain: %v", err)
		return s.blocked
	}
	return nil
}

func (s *Store) readRecorded(path string, value any) error {
	if s.blocked != nil {
		return s.blocked
	}
	if _, err := checkPresence(path); err != nil {
		return err
	}
	return readJSON(path, value)
}

func (s *Store) paths(suffix string) ([]string, error) {
	root, err := filepath.Glob(filepath.Join(s.dir, "*.json"+suffix))
	if err != nil {
		return nil, err
	}
	usage, err := filepath.Glob(filepath.Join(s.dir, "usage", "*", "*.json"+suffix))
	return append(root, usage...), err
}

func (s *Store) checkPresenceLocked() error {
	if s.blocked != nil {
		return s.blocked
	}
	paths, err := s.paths(PresenceSuffix)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if _, err := checkPresence(strings.TrimSuffix(path, PresenceSuffix)); err != nil {
			return err
		}
	}
	return nil
}

// CheckPresence is read-only and runs before product startup recovery. It covers
// usage files too, so losing accounting cannot become an invented zero after boot.
func (s *Store) CheckPresence() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.checkPresenceLocked()
}

// AdoptPresence witnesses existing files after the product's schema preflight.
// Validate the complete candidate set before creating any witnesses. A crash
// between data and its first witness is completed on restart, before serving.
func (s *Store) AdoptPresence() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkPresenceLocked(); err != nil {
		return err
	}
	paths, err := s.paths("")
	if err != nil {
		return err
	}
	var missing []string
	for _, path := range paths {
		var raw json.RawMessage
		if err := readJSON(path, &raw); err != nil {
			return err
		}
		found, err := checkPresence(path)
		if err != nil {
			return err
		}
		if !found {
			missing = append(missing, path)
		}
	}
	for _, path := range missing {
		if err := writeJSON(path+PresenceSuffix, presence{Version: 1}); err != nil {
			s.blocked = fmt.Errorf("store adoption uncertain: %v", err)
			return s.blocked
		}
	}
	return nil
}
