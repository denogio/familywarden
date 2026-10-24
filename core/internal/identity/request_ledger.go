package identity

import (
	"fmt"
	"strings"
	"time"

	"github.com/denogio/familywarden/core/internal/protocol"
	"github.com/denogio/familywarden/core/internal/usign"
)

const maxEpoch int64 = 253402300799 // last second of year 9999; prevents epoch arithmetic overflow

// All ledger access holds Service.mu. Lifecycle transitions, not every sync,
// write flash: a finalized expiry cannot be revived by reboot or clock rollback.
func (s *Service) loadLedger() ([]entry, error) {
	if s.ledger == nil {
		var doc ledgerDoc
		found, err := s.st.ReadDocument(requestsFile, &doc)
		if err != nil {
			return nil, err
		}
		if found {
			if err := validateLedger(doc); err != nil {
				return nil, err
			}
		}
		s.ledger = append([]entry{}, doc.Requests...)
		s.ledgerTime = doc.LastTime
	}
	now := s.Now().Unix()
	if now < s.ledgerTime || now <= 0 || now > maxEpoch-int64(protocol.GrantLifetime/time.Second) {
		return nil, fmt.Errorf("requests: clock outside trusted range")
	}
	next := make([]entry, 0, len(s.ledger))
	changed := false
	for _, e := range s.ledger {
		if now-e.end() >= int64(keepAnswered/time.Second) {
			changed = true
			continue
		}
		if e.expired(now) {
			e.State = "expired"
			changed = true
		}
		if e.State == "approved" && now >= e.NotAfter && !e.GrantExpired {
			e.GrantExpired = true
			changed = true
		}
		next = append(next, e)
	}
	if changed {
		if err := s.saveLedger(next, now); err != nil {
			return nil, err
		}
	}
	return s.ledger, nil
}

// Nothing changes in RAM if persistence fails; a retry can safely repeat it.
func (s *Service) saveLedger(next []entry, now int64) error {
	doc := ledgerDoc{Version: requestsVersion, LastTime: now, Requests: next}
	if err := validateLedger(doc); err != nil {
		return err
	}
	if err := s.st.WriteDocument(requestsFile, doc); err != nil {
		return err
	}
	s.ledger, s.ledgerTime = next, now
	// Delivery observations are RAM-only and must not grow beyond retained grants.
	retained := map[string]bool{}
	for _, e := range next {
		if e.GrantID != "" {
			retained[e.GrantID] = true
		}
	}
	for id := range s.delivered {
		if !retained[id] {
			delete(s.delivered, id)
		}
	}
	return nil
}

func (s *Service) requesterExists(e entry) bool {
	i := s.find(e.Device)
	return i >= 0 && s.devices[i].Profile == e.Profile && s.devices[i].PubKey == e.DeviceKey
}

func validTarget(target string) bool {
	return target == targetTotal || (len(target) <= 255 && validPkg.MatchString(target))
}

func validateLedger(doc ledgerDoc) error {
	if doc.Version != requestsVersion || len(doc.Requests) > maxLedger || doc.LastTime <= 0 || doc.LastTime > maxEpoch {
		return fmt.Errorf("requests: invalid schema, size or clock")
	}
	ids, grants := map[string]bool{}, map[string]bool{}
	for _, e := range doc.Requests {
		_, labelOK := cleanText(e.Label, 64)
		_, noteOK := cleanText(e.Note, 140)
		_, keyErr := usign.ParsePublicKey(e.DeviceKey)
		if !validRequestID.MatchString(e.ID) || ids[e.ID] || !validHash.MatchString(e.Hash) ||
			!validProfile.MatchString(e.Profile) || !validDevice.MatchString(e.Device) || keyErr != nil ||
			!validTarget(e.Target) || !labelOK || !noteOK || e.Minutes < 1 || e.Minutes > protocol.MaxMinutes ||
			e.Received <= 0 || e.Received > doc.LastTime || e.Expires <= e.Received ||
			e.Expires > e.Received+int64(protocol.RequestTTL/time.Second) {
			return fmt.Errorf("requests: invalid request entry %q", e.ID)
		}
		ids[e.ID] = true
		switch e.State {
		case "pending", "expired":
			if e.DecidedBy != "" || e.DecidedAt != 0 || e.Granted != 0 || e.GrantID != "" || e.NotAfter != 0 || e.GrantExpired ||
				(e.State == "expired" && doc.LastTime < e.Expires) {
				return fmt.Errorf("requests: invalid unanswered entry %q", e.ID)
			}
		case "approved", "denied":
			if !(e.DecidedBy == "router" || (strings.HasPrefix(e.DecidedBy, "phone:") && validDevice.MatchString(strings.TrimPrefix(e.DecidedBy, "phone:")))) || e.DecidedAt < e.Received || e.DecidedAt >= e.Expires || e.DecidedAt > doc.LastTime {
				return fmt.Errorf("requests: invalid decision entry %q", e.ID)
			}
			if e.State == "approved" {
				if e.Granted < 1 || e.Granted > e.Minutes || !validGrantID.MatchString(e.GrantID) || grants[e.GrantID] ||
					e.NotAfter != e.DecidedAt+int64(protocol.GrantLifetime/time.Second) || e.NotAfter > maxEpoch ||
					(e.GrantExpired && doc.LastTime < e.NotAfter) {
					return fmt.Errorf("requests: invalid grant entry %q", e.ID)
				}
				grants[e.GrantID] = true
			} else if e.Granted != 0 || e.GrantID != "" || e.NotAfter != 0 || e.GrantExpired {
				return fmt.Errorf("requests: denied entry contains a grant %q", e.ID)
			}
		default:
			return fmt.Errorf("requests: unknown state %q", e.State)
		}
	}
	return nil
}
