// Package service composes the portable FamilyWarden services and local API.
// Host construction, secrets, authentication and socket lifecycle stay outside.
package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/denogio/familywarden/core/internal/configuration"
	"github.com/denogio/familywarden/core/internal/identity"
)

const (
	MaxQuery = 8 << 10
	MaxAPI   = 64 << 10
)

func Handler(id *identity.Service, version string) http.Handler {
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
		q, err := io.ReadAll(io.LimitReader(r.Body, MaxQuery+1))
		if err != nil || len(q) > MaxQuery {
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
		b, err := io.ReadAll(io.LimitReader(r.Body, MaxAPI+1))
		if err != nil || len(b) > MaxAPI {
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
		b, err := io.ReadAll(io.LimitReader(r.Body, MaxAPI+1))
		if err == nil && len(b) > MaxAPI {
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
		case "core_status":
			reply = identity.Reply{"running": true, "core": version}
		case "configuration_export":
			reply = id.ConfigurationExport()
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
		case "profiles":
			reply = id.ProfileDirectory()
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

func WithConfiguration(base http.Handler, cfg *configuration.Authority) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recovery := strings.HasPrefix(r.URL.Path, "/release/")
		if r.Method != http.MethodPost || (!recovery && !strings.HasPrefix(r.URL.Path, "/api/configuration_")) {
			base.ServeHTTP(w, r)
			return
		}
		b, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
		if err != nil || len(b) > 1<<20 {
			writeJSON(w, map[string]any{"error": "malformed configuration request"})
			return
		}
		if recovery {
			writeJSON(w, cfg.Recovery(strings.TrimPrefix(r.URL.Path, "/release/"), b))
		} else if strings.HasPrefix(r.URL.Path, "/api/configuration_sail_") {
			writeJSON(w, map[string]any{"error": "protected release entry point required"})
		} else {
			writeJSON(w, cfg.API(strings.TrimPrefix(r.URL.Path, "/api/"), b))
		}
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
