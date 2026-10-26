package identity

import (
	"sync"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
)

// Pause the first context read without blocking later reads in the adapter.
type adminContextGate struct {
	platform.Adapter
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (g *adminContextGate) Context() (platform.Context, error) {
	first := false
	g.once.Do(func() { first = true })
	if first {
		close(g.entered)
		<-g.release
	}
	return g.Adapter.Context()
}

func (g *adminContextGate) WithContext(fn func(platform.Context) error) error {
	ctx, err := g.Context()
	if err != nil {
		return err
	}
	return fn(ctx)
}

func TestPartialDeviceUpdatesDoNotRestoreStaleOmittedFields(t *testing.T) {
	f := setupFamily(t)
	gate := &adminContextGate{Adapter: f.s.ad, entered: make(chan struct{}), release: make(chan struct{})}
	f.s.ad = gate
	var release sync.Once
	unblock := func() { release.Do(func() { close(gate.release) }) }
	defer unblock()
	rename := make(chan Reply, 1)
	go func() { rename <- f.s.Update(Change{Device: "fw-parent", Name: "Robin phone"}) }()
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("update did not reach the context read")
	}

	// The context check and patch resolution must belong to the final write lock.
	unlocked := f.s.mu.TryLock()
	if unlocked {
		f.s.mu.Unlock()
		t.Error("update read platform state outside the identity transaction")
	}
	admin := make(chan Reply, 1)
	go func() { admin <- f.s.Update(Change{Device: "fw-parent", Administrator: "1"}) }()
	if unlocked {
		// Force the old bug: the admin patch commits before the paused rename
		// writes its stale copy of the omitted administrator field.
		if result := <-admin; result["status"] != "ok" {
			t.Fatal(result)
		}
	}
	unblock()
	if result := <-rename; result["status"] != "ok" {
		t.Fatal(result)
	}
	if !unlocked {
		if result := <-admin; result["status"] != "ok" {
			t.Fatal(result)
		}
	}
	device := find(f.s, "fw-parent")
	if device["name"] != "Robin phone" || device["administrator"] != "1" || device["approves"] != "kid" {
		t.Fatalf("partial updates lost another field: %v", device)
	}
}
