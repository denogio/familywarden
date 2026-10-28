// familywardend is the FamilyWarden core: identity, signed policy and app control
// (plans/app-control.md, decision A6). It listens only on a local unix socket; the
// OpenWrt adapter (rpcd/uhttpd) is the only thing that talks to it.
package main

import (
	"bytes"
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
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	_ "time/tzdata" // OpenWrt ships no zoneinfo; day boundaries need the router's timezone (D8)

	"github.com/denogio/familywarden/core/internal/openwrt"
	"github.com/denogio/familywarden/core/internal/service"
	"github.com/denogio/familywarden/core/internal/store"
	"path/filepath"
)

var version = "dev" // set with -ldflags "-X main.version=..."

const (
	maxQuery = service.MaxQuery
	maxAPI   = service.MaxAPI
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// listen takes over socket only if it is absent or a stale socket. A regular file
// (a typo in -socket) is never deleted, and a live socket means another instance
// is running. The separate lifetime writer lock protects the data directory
// before recovery, including when two processes use different socket paths.
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

func client(socket string) *http.Client {
	return &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}}
}

// cgiPhone is the OpenWrt transport (uhttpd CGI on port 7700): it forwards
// QUERY_STRING and prints the CGI reply. Core down = 503 and no body: phones keep
// their last trusted policy, never get an unsigned or weaker one.
func cgiPhone(socket string, out io.Writer) {
	status := map[int]string{200: "200 OK", 400: "400 Bad Request", 403: "403 Forbidden", 500: "500 Internal Server Error"}
	code, body := 503, []byte(nil)
	resp, err := client(socket).Post("http://core/phone", "text/plain", strings.NewReader(os.Getenv("QUERY_STRING")))
	if err == nil {
		body, err = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if err == nil {
			code = resp.StatusCode
		}
	}
	line, known := status[code]
	if !known {
		line, body = "503 Service Unavailable", nil
	}
	fmt.Fprintf(out, "Status: %s\nContent-Type: text/plain\nCache-Control: no-store\n\n", line)
	_, _ = out.Write(body)
}

// cgiApp forwards a signed upload (POST body) for ?op=request|decision|usage.
func cgiApp(socket string, in io.Reader, out io.Writer) {
	status := map[int]string{200: "200 OK", 400: "400 Bad Request", 403: "403 Forbidden", 500: "500 Internal Server Error"}
	code, body := 400, []byte(nil)
	op, hasPrefix := strings.CutPrefix(os.Getenv("QUERY_STRING"), "op=")
	if os.Getenv("REQUEST_METHOD") == "POST" && hasPrefix && (op == "request" || op == "decision" || op == "usage" || op == "app") {
		b, err := io.ReadAll(io.LimitReader(in, maxAPI+1))
		code = 503
		if err == nil && len(b) <= maxAPI {
			resp, err := client(socket).Post("http://core/app/"+op, "text/plain", bytes.NewReader(b))
			if err == nil {
				body, err = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
				resp.Body.Close()
				if err == nil {
					code = resp.StatusCode
				}
			}
		} else if err == nil {
			code = 400
		}
	}
	line, known := status[code]
	if !known {
		line, body = "503 Service Unavailable", nil
	}
	fmt.Fprintf(out, "Status: %s\nContent-Type: text/plain\nCache-Control: no-store\n\n", line)
	_, _ = out.Write(body)
}

// withoutSession drops the ubus_rpc_session that uhttpd adds to calls from LuCI.
// rpcd has already authorized that session; the core API never sees transport fields,
// so its strict decoders stay strict. Anything that is not an object passes unchanged.
func withoutSession(b []byte) []byte {
	// Do not let transport cleanup normalize duplicate fields or oversized input
	// before the core's strict decoders can reject them.
	if len(b) > maxAPI || !utf8.Valid(b) || !json.Valid(b) {
		return b
	}
	d := json.NewDecoder(bytes.NewReader(b))
	if token, _ := d.Token(); token != json.Delim('{') {
		return b
	}
	m := make(map[string]json.RawMessage)
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return b
		}
		key := token.(string)
		if _, duplicate := m[key]; duplicate {
			return b
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return b
		}
		m[key] = value
	}
	if _, ok := m["ubus_rpc_session"]; !ok {
		return b
	}
	delete(m, "ubus_rpc_session")
	out, err := json.Marshal(m)
	if err != nil {
		return b
	}
	return out
}

// api is the rpcd shim: JSON on stdin, one JSON object on stdout.
func api(socket, method string, in io.Reader, out io.Writer) {
	limit := maxAPI
	if strings.HasPrefix(method, "configuration_") {
		limit = 1 << 20
	}
	localRequest(socket, "/api/"+method, limit, in, out)
}

func localRequest(socket, path string, limit int, in io.Reader, out io.Writer) {
	b, _ := io.ReadAll(io.LimitReader(in, int64(limit+1)))
	b = withoutSession(b)
	resp, err := client(socket).Post("http://core"+path, "application/json", bytes.NewReader(b))
	if err == nil {
		defer resp.Body.Close()
		var v map[string]any
		if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&v); err == nil {
			_ = json.NewEncoder(out).Encode(v)
			return
		}
	}
	_ = json.NewEncoder(out).Encode(map[string]any{"error": "core not reachable"})
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
	shell := flag.String("shell", "/usr/libexec/rpcd/familywarden", "OpenWrt adapter: the familywarden shell plugin")
	routerKey := flag.String("router-key", "/etc/familywarden/mast.key", "OpenWrt adapter: router usign secret key")
	trustedPub := flag.String("trusted-pub", "", "backup inspection: externally trusted usign public key file")
	flag.Parse()
	switch flag.Arg(0) {
	case "backup":
		if flag.NArg() != 2 {
			log.Fatal("usage: familywardend [flags] backup OUTPUT.tar.gz (offline only)")
		}
		ad := openwrt.Adapter{Shell: *shell, KeyFile: *routerKey, ConfigFile: filepath.Join(*dataDir, "configuration.json")}
		m, err := exportBackup(ad, *dataDir, flag.Arg(1), version)
		if err != nil {
			log.Fatal(err)
		}
		if err := backupSummary(os.Stdout, m); err != nil {
			log.Fatal(err)
		}
		return
	case "backup-inspect":
		if flag.NArg() != 2 {
			log.Fatal("usage: familywardend -trusted-pub PIN.pub backup-inspect INPUT.tar.gz")
		}
		if *trustedPub == "" {
			log.Fatal("backup inspection requires -trusted-pub from an external trusted source")
		}
		if err := inspectBackup(flag.Arg(1), *trustedPub, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	case "app-canon":
		if err := appCanon(os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	case "agh-services":
		if err := aghServices(os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	case "agh-sync":
		if err := aghSync(os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	case "status":
		status(*socket, os.Stdout)
		return
	case "cgi-phone":
		cgiPhone(*socket, os.Stdout)
		return
	case "cgi-app":
		cgiApp(*socket, os.Stdin, os.Stdout)
		return
	case "api":
		api(*socket, flag.Arg(1), os.Stdin, os.Stdout)
		return
	case "release":
		localRequest(*socket, "/release/"+flag.Arg(1), maxAPI, os.Stdin, os.Stdout)
		return
	}

	if flag.NArg() != 0 {
		log.Fatal("unknown command; backup inspection is not restore authorization")
	}
	writer, err := acquireWriter(*dataDir)
	if err != nil {
		log.Fatalf("exclusive core startup: %v", err)
	}
	defer writer.Close()
	st, err := store.Open(*dataDir)
	if err != nil {
		log.Fatalf("data directory %s: %v", *dataDir, err)
	}
	// The adapter may not be ready yet at boot: retry instead of exiting, so procd's
	// respawn limit can never leave phone sync down. Until then nothing listens and
	// phones get 503, keeping their last trusted policy.
	var app *service.Service
	ad := openwrt.Adapter{Shell: *shell, KeyFile: *routerKey, ConfigFile: filepath.Join(*dataDir, "configuration.json")}
	for wait := time.Second; ; wait = min(2*wait, time.Minute) {
		app, err = service.New(ad, ad, st, version)
		if err == nil {
			break
		}
		log.Printf("identity not ready (retrying in %v): %v", wait, err)
		time.Sleep(wait)
	}
	defer app.Close()
	l, err := listen(*socket)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	srv := &http.Server{Handler: app.Handler, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownDone := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownDone <- srv.Shutdown(shutdown)
	}()
	log.Printf("familywardend %s listening on %s", version, *socket)
	if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	// Serve returns when the listener closes, not when in-flight writes finish.
	// Keep writer ownership until draining completes. On timeout exit directly:
	// the kernel releases the lease when the process and its writers are gone.
	if err := <-shutdownDone; err != nil {
		log.Fatalf("core shutdown did not drain: %v", err)
	}
}
