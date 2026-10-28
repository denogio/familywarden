package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
)

// GenerationDocument is a reserved, store-owned document. It advances BEFORE
// every other top-level document write, so a consistent offline snapshot never
// pairs older household content with a generation that a phone already saw for
// newer content. Usage days are excluded: phones re-raise their own counters.
const GenerationDocument = "generation.json"

const maxLineage = 16

var validEpoch = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Ancestor is a fork point: the epoch's counter when a restore left it.
type Ancestor struct {
	Epoch   string `json:"epoch"`
	Counter int64  `json:"counter"`
}

// Generation is (epoch, counter) plus bounded restore lineage. A phone accepts
// a reply only when it continues the phone's last epoch at or beyond its counter,
// or when a lineage fork point does. An older restored backup cannot catch up by
// later writes: its fork point for the phone's epoch is permanently too low.
type Generation struct {
	Version int        `json:"version"`
	Epoch   string     `json:"epoch"`
	Counter int64      `json:"counter"`
	Lineage []Ancestor `json:"lineage"`
}

func (g Generation) valid() error {
	if g.Version != 1 || !validEpoch.MatchString(g.Epoch) || g.Counter < 0 || g.Counter > 1<<53 || len(g.Lineage) > maxLineage {
		return errors.New("unsupported household generation")
	}
	seen := map[string]bool{g.Epoch: true}
	for _, a := range g.Lineage {
		if !validEpoch.MatchString(a.Epoch) || seen[a.Epoch] || a.Counter < 0 || a.Counter > 1<<53 {
			return errors.New("invalid household generation lineage")
		}
		seen[a.Epoch] = true
	}
	return nil
}

func newEpoch() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (s *Store) generationLocked() (Generation, bool, error) {
	var g Generation
	err := s.readRecorded(filepath.Join(s.dir, GenerationDocument), &g)
	if errors.Is(err, os.ErrNotExist) {
		return g, false, nil
	}
	if err != nil {
		return g, true, err
	}
	return g, true, g.valid()
}

// Generation returns the current value; absent means first use (counter 0, no
// epoch yet). Read it AFTER the content it vouches for.
func (s *Store) Generation() (Generation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, _, err := s.generationLocked()
	if g.Version == 0 {
		g = Generation{Version: 1, Lineage: []Ancestor{}}
	}
	return g, err
}

// CheckGeneration is the read-only startup preflight.
func (s *Store) CheckGeneration() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _, err := s.generationLocked()
	return err
}

func (s *Store) advanceLocked() error {
	g, found, err := s.generationLocked()
	if err != nil {
		return err
	}
	if !found {
		if g.Epoch, err = newEpoch(); err != nil {
			return err
		}
		g.Version, g.Lineage = 1, []Ancestor{}
	}
	g.Counter++
	return s.writeRecorded(filepath.Join(s.dir, GenerationDocument), g)
}

// Fork starts a new epoch for a restored household, remembering where each
// earlier epoch stopped. Only the offline restore calls it, on staged state.
func (s *Store) Fork() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, found, err := s.generationLocked()
	if err != nil {
		return err
	}
	epoch, err := newEpoch()
	if err != nil {
		return err
	}
	next := Generation{Version: 1, Epoch: epoch, Lineage: []Ancestor{}}
	if found {
		next.Lineage = append([]Ancestor{{g.Epoch, g.Counter}}, g.Lineage...)
		if len(next.Lineage) > maxLineage {
			next.Lineage = next.Lineage[:maxLineage]
		}
	}
	return s.writeRecorded(filepath.Join(s.dir, GenerationDocument), next)
}
