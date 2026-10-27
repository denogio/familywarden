package openwrt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
)

// WithContext holds the existing state lock through the core's durable mutation.
// Only fd 3 is inherited: the shell reads through that same lock instead of
// opening a second lock and deadlocking against its own parent. Taking only the
// state lock is safe because callbacks never acquire ticket/bonus/apply locks.
func (a Adapter) WithContext(fn func(platform.Context) error) error {
	path := a.LockFile
	if path == "" {
		path = "/var/lock/familywarden.lock"
	}
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := acquireStateLock(ctx, lock); err != nil {
		return fmt.Errorf("state transaction: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	var c platform.Context
	if err := a.runLocked(&c, lock, "core-context", appFormat, "held-v1"); err != nil {
		return err
	}
	if err := compileContext(&c); err != nil {
		return err
	}
	return fn(c)
}

// withStateLocks shares the established ticket-before-state lock order between
// configuration changes, manual network actions and grant ticket repair.
func (a Adapter) withStateLocks(fn func(state, ticket *os.File) error) error {
	path := a.LockFile
	if path == "" {
		path = "/var/lock/familywarden.lock"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ticket, err := os.OpenFile(path+".ticket", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer ticket.Close()
	if err := acquireStateLock(ctx, ticket); err != nil {
		return err
	}
	defer syscall.Flock(int(ticket.Fd()), syscall.LOCK_UN)
	state, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer state.Close()
	if err := acquireStateLock(ctx, state); err != nil {
		return err
	}
	defer syscall.Flock(int(state.Fd()), syscall.LOCK_UN)
	return fn(state, ticket)
}

func acquireStateLock(ctx context.Context, lock *os.File) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}
