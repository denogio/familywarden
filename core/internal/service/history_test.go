package service

import (
	"strings"
	"testing"
	"time"
)

func TestPortableSignedHistoryReadsCurrentScopeWithoutMutation(t *testing.T) {
	f := newPortableFixture(t)
	child := f.phone("fw-alex", "alex")
	parent := f.phone("fw-parent", "robin", "alex")
	day := f.now().Format(time.DateOnly)
	if r := f.upload(child, "usage", map[string]any{"type": "usage", "v": 1, "device": child.id, "profile": "alex", "day": day,
		"seq": 1, "total": 120, "apps": map[string]int{"com.example.video": 120}}); r["state"] != "stored" {
		t.Fatal(r)
	}
	query := map[string]any{"type": "history", "v": 1, "device": parent.id, "profile": "alex", "day": day, "created": f.now().Unix(), "nonce": strings.Repeat("a", 64)}
	before := f.configuration().Revision
	for i := 0; i < 2; i++ {
		generation, err := f.app.Identity.Generation()
		if err != nil {
			t.Fatal(err)
		}
		r := f.upload(parent, "history", query)
		after, err := f.app.Identity.Generation()
		if err != nil || after.Counter != generation.Counter || after.Epoch != generation.Epoch {
			t.Fatal("history advanced durable authority", err)
		}
		if r["reported"] != true || r["total"] != float64(120) {
			t.Fatal(r)
		}
		devices := r["devices"].([]any)
		if objectField(t, devices[0])["received"] != float64(f.now().Unix()) {
			t.Fatal(r)
		}
		if f.configuration().Revision != before {
			t.Fatal("read changed configuration")
		}
		f.restart() // real store, same signed read and no extra generation/ledger write
	}
	f.api("device_update", map[string]any{"device": parent.id, "profile": "robin", "name": "Parent phone", "approves": ""})
	if r := f.upload(parent, "history", query); r["error"] != "not_parent" {
		t.Fatal("old signed scope read history", r)
	}
}
