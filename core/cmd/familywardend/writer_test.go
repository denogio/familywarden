package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestWriterLeasePrivateAndStable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".writer.lock")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}
	first, err := acquireWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	info, err := first.Stat()
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("lock permissions", info, err)
	}
	alias := filepath.Join(t.TempDir(), "same-data")
	if err := os.Symlink(dir, alias); err != nil {
		t.Fatal(err)
	}
	if other, err := acquireWriter(alias); err == nil {
		other.Close()
		t.Fatal("same directory acquired through another path")
	}
	first.Close()
	next, err := acquireWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	after, err := next.Stat()
	if err != nil || !os.SameFile(info, after) {
		t.Fatal("writer lock inode replaced", err)
	}
	next.Close()
	linkDir := t.TempDir()
	if err := os.Symlink(path, filepath.Join(linkDir, ".writer.lock")); err != nil {
		t.Fatal(err)
	}
	if other, err := acquireWriter(linkDir); err == nil {
		other.Close()
		t.Fatal("followed a writer-lock symlink")
	}
}

func TestWriterLeaseProcess(t *testing.T) {
	// Child processes run either the lease holder or the actual daemon startup.
	// All paths, including adapter/key/socket flags, stay inside the temp fixture.
	if dir := os.Getenv("FW_TEST_WRITER_DIR"); dir != "" {
		mode := os.Getenv("FW_TEST_WRITER_MODE")
		if mode == "daemon" || mode == "serve" {
			flag.CommandLine = flag.NewFlagSet("familywardend", flag.ExitOnError)
			os.Args = []string{"familywardend", "-data", dir, "-socket", filepath.Join(dir, "other.sock"),
				"-shell", filepath.Join(dir, "no-adapter"), "-router-key", filepath.Join(dir, "no-key")}
			main()
			if mode == "serve" {
				return
			}
			t.Fatal("second daemon unexpectedly started")
		}
		lease, err := acquireWriter(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer lease.Close()
		fmt.Fprintln(os.Stdout, "writer-ready")
		io.Copy(io.Discard, os.Stdin) // parent keeps this pipe open until exit/kill
		return
	}
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, kill := range []bool{false, true} {
		t.Run(fmt.Sprintf("kill=%t", kill), func(t *testing.T) {
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, bin, "-test.run=^TestWriterLeaseProcess$")
			child.Env = append(os.Environ(), "FW_TEST_WRITER_DIR="+dir, "FW_TEST_WRITER_MODE=hold")
			input, err := child.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			output, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { child.Process.Kill(); child.Wait() })
			if line, err := bufio.NewReader(output).ReadString('\n'); err != nil || line != "writer-ready\n" {
				t.Fatal("child did not acquire writer lease", line, err)
			}
			probe := exec.CommandContext(ctx, bin, "-test.run=^TestWriterLeaseProcess$")
			probe.Env = append(os.Environ(), "FW_TEST_WRITER_DIR="+dir, "FW_TEST_WRITER_MODE=daemon")
			data, err := probe.CombinedOutput()
			if err == nil || !strings.Contains(string(data), "exclusive core startup") {
				t.Fatal("second daemon reached startup instead of rejecting ownership", err, string(data))
			}
			if _, err := os.Stat(filepath.Join(dir, "usage")); !os.IsNotExist(err) {
				t.Fatal("second daemon opened/mutated the store before exclusion", err)
			}
			if kill {
				if err := child.Process.Kill(); err != nil {
					t.Fatal(err)
				}
			} else {
				input.Close()
			}
			if err := child.Wait(); !kill && err != nil {
				t.Fatal(err)
			}
			lease, err := acquireWriter(dir)
			if err != nil {
				t.Fatal("process exit left a stale writer lock", err)
			}
			lease.Close()
		})
	}
}

func TestDaemonReleasesWriterAfterShutdown(t *testing.T) {
	dir := t.TempDir()
	// A fresh legacy household with an already imported empty registry needs no
	// adapter calls to start. Missing adapter/key paths prevent any live access.
	if err := os.WriteFile(filepath.Join(dir, "devices.json"), []byte(`{"version":1,"devices":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-test.run=^TestWriterLeaseProcess$")
	cmd.Env = append(os.Environ(), "FW_TEST_WRITER_DIR="+dir, "FW_TEST_WRITER_MODE=serve")
	logs, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	scanner := bufio.NewScanner(logs)
	ready := false
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "listening on") {
			ready = true
			break
		}
	}
	if !ready {
		t.Fatal("daemon did not listen", scanner.Err())
	}
	resp, err := client(filepath.Join(dir, "other.sock")).Get("http://core/version")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal("daemon did not serve", resp.StatusCode)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal("graceful shutdown failed", err)
	}
	lease, err := acquireWriter(dir)
	if err != nil {
		t.Fatal("shutdown did not release writer ownership", err)
	}
	lease.Close()
}
