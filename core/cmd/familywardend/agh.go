package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/denogio/familywarden/core/internal/agh"
)

// aghSync is the stateless AdGuard Home reconciler: the platform passes the
// desired DNS profiles and stores the result (acks, owned ids, allocations).
func aghSync(in io.Reader, out io.Writer) error {
	const max = 1 << 20
	b, err := io.ReadAll(io.LimitReader(in, max+1))
	if err != nil {
		return err
	}
	if len(b) > max || !utf8.Valid(b) {
		return fmt.Errorf("invalid or oversized DNS sync input")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	var v agh.Input
	if err := d.Decode(&v); err != nil {
		return err
	}
	// Like uclient-fetch --no-proxy: AGH is on the LAN, never via a proxy.
	hc := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 10 * time.Second}
	return json.NewEncoder(out).Encode(agh.Sync(v, hc))
}
