// familywardend is the FamilyWarden core: identity, signed policy and app control
// (plans/app-control.md, decision A6). It listens only on a local unix socket; the
// OpenWrt adapter (rpcd/uhttpd) is the only thing that talks to it.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
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

func main() {
	dataDir := flag.String("data", "/etc/familywarden/core", "directory for durable state")
	socket := flag.String("socket", "/var/run/familywarden/core.sock", "unix socket to listen on")
	flag.Parse()

	st, err := store.Open(*dataDir)
	if err != nil {
		log.Fatalf("data directory %s: %v", *dataDir, err)
	}
	if err := os.Remove(*socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("stale socket %s: %v", *socket, err)
	}
	l, err := net.Listen("unix", *socket)
	if err != nil {
		log.Fatalf("listen %s: %v", *socket, err)
	}
	// Root-only: the adapter runs as root; nothing else on the router may call the core.
	if err := os.Chmod(*socket, 0o600); err != nil {
		log.Fatalf("chmod %s: %v", *socket, err)
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
