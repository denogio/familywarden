package openwrt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/denogio/familywarden/core/internal/platform"
)

func transactionAdapter(t *testing.T, raw string) Adapter {
	t.Helper()
	dir := t.TempDir()
	a := Adapter{Shell: filepath.Join(dir, "adapter"), LockFile: filepath.Join(dir, "state.lock")}
	// Exercise real inherited-descriptor semantics, not a mock lock boolean.
	script := fmt.Sprintf("#!/bin/sh\n[ \"$*\" = 'core-context go-apps-v3 held-v1' ] || exit 1\n[ /proc/self/fd/3 -ef %q ] || exit 2\nflock -n -x 3 || exit 3\nprintf '%%s' '%s'\n", a.LockFile, raw)
	if err := os.WriteFile(a.Shell, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return a
}

const transactionContext = `{"app_format":"go-apps-v3","pact_captain":"alex","profiles":{"alex":{"name":"Alex","app_input":{"version":1,"enabled":"1","apps":[{"package":"com.example.video","minutes":"10"}]}}}}`

func probeStateLock(t *testing.T, path string, held bool) {
	t.Helper()
	other, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	err = syscall.Flock(int(other.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if held && !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("state lock not held through callback: %v", err)
	}
	if !held && err != nil {
		t.Fatalf("state lock leaked: %v", err)
	}
}

func TestMutationContextHoldsInheritedLockThroughCallback(t *testing.T) {
	for _, callbackError := range []bool{false, true} {
		t.Run(fmt.Sprint(callbackError), func(t *testing.T) {
			a := transactionAdapter(t, transactionContext)
			wantErr := errors.New("save failed")
			calls := 0
			err := a.WithContext(func(c platform.Context) error {
				calls++
				probeStateLock(t, a.LockFile, true)
				if c.PactCaptain != "alex" || c.Profiles["alex"].AppsRev == "" {
					t.Fatal("uncompiled or incorrect snapshot", c)
				}
				if callbackError {
					return wantErr
				}
				return nil
			})
			if calls != 1 || (callbackError && !errors.Is(err, wantErr)) || (!callbackError && err != nil) {
				t.Fatal(calls, err)
			}
			probeStateLock(t, a.LockFile, false)
		})
	}
}

func TestMutationContextRefusesBadSnapshotAndReleasesLock(t *testing.T) {
	for _, raw := range []string{`not-json`, `{"error":"unavailable"}`, `{"app_format":"go-apps-v2","profiles":{}}`, `{"app_format":"go-apps-v3","profiles":{"alex":{}}}`} {
		a := transactionAdapter(t, raw)
		called := false
		err := a.WithContext(func(platform.Context) error { called = true; return nil })
		if err == nil || called {
			t.Fatal("mutation executed without compatible context", raw, err)
		}
		probeStateLock(t, a.LockFile, false)
	}
}

func TestStateLockCancellationDoesNotWaitOrStealLock(t *testing.T) {
	a := transactionAdapter(t, transactionContext)
	owner, err := os.OpenFile(a.LockFile, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err := syscall.Flock(int(owner.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	contender, err := os.OpenFile(a.LockFile, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer contender.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := acquireLock(ctx, contender, false); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	probeStateLock(t, a.LockFile, true)
}
