package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/denogio/familywarden/core/internal/agh"
)

// aghSync is the stateless AdGuard Home reconciler: the platform passes the
// desired DNS profiles and stores the result (acks, owned ids, allocations).
func aghSync(in io.Reader, out io.Writer) error {
	var v agh.Input
	if err := decodeAGH(in, &v, 1<<20); err != nil {
		return err
	}
	if err := validateAGHEndpoint(v.URL, v.Auth); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(agh.Sync(v, lanClient()))
}

// Like uclient-fetch --no-proxy: AGH is on the LAN, never via a proxy.
func lanClient() *http.Client {
	return agh.NewHTTPClient()
}

func decodeAGH(in io.Reader, v any, max int64) error {
	b, err := io.ReadAll(io.LimitReader(in, max+1))
	if err != nil {
		return err
	}
	if int64(len(b)) > max || !utf8.Valid(b) || len(bytes.TrimSpace(b)) == 0 || bytes.TrimSpace(b)[0] != '{' {
		return fmt.Errorf("invalid or oversized AGH input")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing AGH input")
	}
	return nil
}

func validateAGHEndpoint(raw, auth string) error {
	if !agh.ValidEndpoint(raw) || auth == "" {
		return fmt.Errorf("invalid AGH endpoint or missing credentials")
	}
	return nil
}

// aghServices prints {"services":[...]} or {"error":...} for LuCI's picker.
func aghServices(in io.Reader, out io.Writer) error {
	var v struct {
		URL  string `json:"url"`
		Auth string `json:"auth"`
	}
	if err := decodeAGH(in, &v, 64<<10); err != nil {
		return err
	}
	if err := validateAGHEndpoint(v.URL, v.Auth); err != nil {
		return err
	}
	services, err := agh.Services(v.URL, v.Auth, lanClient())
	if err != nil {
		return json.NewEncoder(out).Encode(map[string]string{"error": "AdGuard Home unreachable"})
	}
	return json.NewEncoder(out).Encode(map[string]any{"services": services})
}
