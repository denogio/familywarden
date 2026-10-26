package configuration

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// failWrite models a full disk at either the initial authority journal or the
// final import-complete write. A durable ownership marker must never fall back.
type failWrite struct {
	memory
	at, writes int
}

func (s *failWrite) WriteDocument(name string, v any) error {
	s.writes++
	if s.writes == s.at {
		return errors.New("interrupted import write")
	}
	return s.memory.WriteDocument(name, v)
}

func TestInterruptedImportRecovery(t *testing.T) {
	for _, phase := range []string{"initial-write", "platform-apply", "completion-write"} {
		t.Run(phase, func(t *testing.T) {
			b := &fakeBackend{current: sample()}
			_ = b.current.Seal()
			st := &failWrite{}
			switch phase {
			case "initial-write":
				st.at = 1
			case "platform-apply":
				b.fail = 1
			case "completion-write":
				st.at = 2
			}
			a, err := New(b, b, st)
			if err != nil {
				t.Fatal(err)
			}
			revision := b.current.Revision
			if r := a.API("configuration_import", request(map[string]any{"revision": revision})); r["error"] == nil {
				t.Fatal("interrupted import reported success", r)
			}
			if phase == "initial-write" {
				if b.applies != 0 || len(st.data) != 0 || a.API("configuration_status", nil)["owner"] != "legacy" {
					t.Fatal("projection changed before a durable ownership journal")
				}
				return
			}
			if len(st.data) == 0 || a.API("configuration_export", nil)["error"] == nil {
				t.Fatal("interrupted durable import fell back to legacy")
			}
			// Restart completes the exact durable input, not newly edited UCI.
			b.current.Current.Profiles[0].Policy = "allow"
			st.at = 0
			recovered, err := New(b, b, st)
			if err != nil {
				t.Fatal(err)
			}
			d, found, err := recovered.load()
			if err != nil || !found || d.NeedsApply || d.Configuration.Revision != revision || b.current.Current.Profiles[0].Policy != "scheduled" {
				t.Fatal("restart failed to finish the durable import", d, err)
			}
			if recovered.API("configuration_import", []byte(`{}`))["already_imported"] != true {
				t.Fatal("recovered import is not idempotent")
			}
		})
	}
}

func TestCorruptRollbackSnapshotNeverBecomesAuthority(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(map[bool]string{false: "deadline", true: "restart"}[restart], func(t *testing.T) {
			a, b, s := setup(t)
			activate(t, a, b)
			okResult(t, a.API("configuration_apply", candidate(a, "Robin")))
			// Corruption can remain schema-valid while changing a restriction. The
			// previous configuration's sealed revision must be checked independently.
			var d document
			if err := json.Unmarshal(s.data, &d); err != nil {
				t.Fatal(err)
			}
			d.Pending.Before.Current.Profiles[0].Policy = "allow"
			s.data = request(d)
			before := append([]byte{}, s.data...)
			applies := b.applies
			if restart {
				if _, err := New(b, b, s); err == nil {
					t.Fatal("corrupt rollback snapshot accepted at restart")
				}
			} else {
				a.Now = func() time.Time { return time.Unix(1800000200, 0).UTC() }
				if _, err := a.Context(); err == nil {
					t.Fatal("corrupt rollback snapshot applied at deadline")
				}
			}
			if b.applies != applies || string(s.data) != string(before) {
				t.Fatal("invalid recovery journal changed the platform or store")
			}
		})
	}
}
