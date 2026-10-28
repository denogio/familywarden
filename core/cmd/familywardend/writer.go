package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// acquireWriter serializes daemon lifetimes, not individual transactions. The
// socket is claimed too late to prevent a second process from running recovery.
// Keep this native lifecycle lock outside the portable product services. Never
// unlink it: replacing the inode would allow a second independent flock owner.
func acquireWriter(dir string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, ".writer.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = fmt.Errorf("writer lock is not a regular file")
	}
	if err == nil {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	}
	if err == nil {
		err = f.Chmod(0600)
	}
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("data directory writer lock: %w", err)
	}
	// Close (including on process death) releases the kernel lock; the file stays.
	return f, nil
}
