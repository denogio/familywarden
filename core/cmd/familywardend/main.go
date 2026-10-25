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

	"github.com/denogio/familywarden/core/internal/identity"
	"github.com/denogio/familywarden/core/internal/openwrt"
	"github.com/denogio/familywarden/core/internal/store"
)

var version = "dev" // set with -ldflags "-X main.version=..."

const (
	maxQuery = 8 << 10
	maxAPI   = 64 << 10
)

func handler(st *store.Store, id *identity.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"core": version})
	})
	if id == nil {
		return mux
	}
	// Phone sync: body = the CGI query string; reply = status + signed body.
	mux.HandleFunc("POST /phone", func(w http.ResponseWriter, r *http.Request) {
		q, err := io.ReadAll(io.LimitReader(r.Body, maxQuery+1))
		if err != nil || len(q) > maxQuery {
			w.WriteHeader(identity.StatusMalformed)
			return
		}
		code, body := id.Phone(string(q))
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(code)
		_, _ = w.Write(body)
	})
	// Signed uploads from phones (requests, decisions); reply = status + signed body.
	mux.HandleFunc("POST /app/{op}", func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(io.LimitReader(r.Body, maxAPI+1))
		if err != nil || len(b) > maxAPI {
			w.WriteHeader(identity.StatusMalformed)
			return
		}
		code, body := id.App(r.PathValue("op"), b)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(code)
		_, _ = w.Write(body)
	})
	// Administrator API (LuCI via the rpcd shim): always one JSON object.
	mux.HandleFunc("POST /api/{method}", func(w http.ResponseWriter, r *http.Request) {
		var c identity.Change
		var a identity.Answer
		var ad identity.AppDecisionInput
		b, err := io.ReadAll(io.LimitReader(r.Body, maxAPI+1))
		if err == nil && len(b) > maxAPI {
			err = errors.New("too large")
		}
		method := r.PathValue("method")
		if err == nil && len(bytes.TrimSpace(b)) > 0 {
			if method == "request_answer" {
				a, err = identity.DecodeAnswer(b)
			} else if method == "app_decision" {
				ad, err = identity.DecodeAppDecision(b)
			} else {
				err = json.Unmarshal(b, &c)
			}
		}
		if err != nil {
			writeJSON(w, identity.Reply{"error": "malformed request"})
			return
		}
		var reply identity.Reply
		switch method {
		case "requests":
			reply = id.Requests()
		case "usage":
			reply = id.UsageReport()
		case "new_apps":
			reply = id.NewApps()
		case "app_decision":
			reply = id.AppDecision(ad)
		case "request_answer":
			reply = id.AnswerRequest(a)
		case "devices":
			reply = id.Devices()
		case "device_approve":
			reply = id.Approve(c)
		case "device_update":
			reply = id.Update(c)
		case "device_reject":
			reply = id.Reject(c.Device)
		case "device_delete":
			reply = id.Delete(c.Device)
		case "device_allow_removal":
			reply = id.AllowRemoval(c.Device)
		default:
			reply = identity.Reply{"error": "unknown method"}
		}
		writeJSON(w, reply)
	})
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
	b, _ := io.ReadAll(io.LimitReader(in, maxAPI+1))
	b = withoutSession(b)
	resp, err := client(socket).Post("http://core/api/"+method, "application/json", bytes.NewReader(b))
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
	flag.Parse()
	switch flag.Arg(0) {
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
	}

	st, err := store.Open(*dataDir)
	if err != nil {
		log.Fatalf("data directory %s: %v", *dataDir, err)
	}
	// The adapter may not be ready yet at boot: retry instead of exiting, so procd's
	// respawn limit can never leave phone sync down. Until then nothing listens and
	// phones get 503, keeping their last trusted policy.
	var id *identity.Service
	for wait := time.Second; ; wait = min(2*wait, time.Minute) {
		if id, err = identity.New(openwrt.Adapter{Shell: *shell, KeyFile: *routerKey}, st); err == nil {
			break
		}
		log.Printf("identity not ready (retrying in %v): %v", wait, err)
		time.Sleep(wait)
	}
	id.Usage = st
	l, err := listen(*socket)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	srv := &http.Server{Handler: handler(st, id), ReadHeaderTimeout: 5 * time.Second}
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
