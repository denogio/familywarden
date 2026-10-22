// familywardend is the FamilyWarden core: identity, signed policy and app control
// (plans/app-control.md, decision A6). It listens only on a local unix socket; the
// OpenWrt adapter (rpcd/uhttpd) is the only thing that talks to it.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "time/tzdata" // OpenWrt ships no zoneinfo; day boundaries need the router's timezone (D8)

	"github.com/denogio/familywarden/core/internal/store"
)

var version = "dev" // set with -ldflags "-X main.version=..."

func handler(st *store.Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"core": version})
	})
	_ = st // endpoints using the store arrive with the first vertical slice (P8)
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// listen takes over socket only if it is absent or a stale socket. A regular file
// (a typo in -socket) is never deleted, and a live socket means another instance
// is running: two cores must never share one data directory.
func listen(socket string) (net.Listener, error) {
	if fi, err := os.Lstat(socket); err == nil {
		if fi.Mode().Type() != os.ModeSocket {
			return nil, fmt.Errorf("%s exists and is not a socket", socket)
		}
		if c, err := net.DialTimeout("unix", socket, time.Second); err == nil {
			c.Close()
			return nil, fmt.Errorf("another familywardend is listening on %s", socket)
		}
		if err := os.Remove(socket); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	l, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	// Root-only: the adapter runs as root; nothing else on the router may call the core.
	if err := os.Chmod(socket, 0o600); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

// status is the rpcd bridge (`ubus call familywarden-core status`): always one JSON
// object, also when the core is down, so LuCI can show "not running" truthfully.
func status(socket string, out io.Writer) {
	c := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}}
	reply := map[string]any{"running": false}
	var v struct {
		Core string `json:"core"`
	}
	resp, err := c.Get("http://core/version")
	if err == nil {
		err = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&v)
		resp.Body.Close()
	}
	if err != nil {
		reply["error"] = "core not reachable"
	} else {
		reply["running"], reply["core"] = true, v.Core
	}
	_ = json.NewEncoder(out).Encode(reply)
}

func main() {
	dataDir := flag.String("data", "/etc/familywarden/core", "directory for durable state")
	socket := flag.String("socket", "/var/run/familywarden-core/core.sock", "unix socket to listen on")
	flag.Parse()
	if flag.Arg(0) == "status" {
		status(*socket, os.Stdout)
		return
	}

	st, err := store.Open(*dataDir)
	if err != nil {
		log.Fatalf("data directory %s: %v", *dataDir, err)
	}
	l, err := listen(*socket)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	srv := &http.Server{Handler: handler(st), ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("familywardend %s listening on %s", version, *socket)
	if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
