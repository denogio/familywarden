package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/denogio/familywarden/core/internal/rules"
)

// appCanon is a stateless compiler for the adapter's pact-snapshot projection.
// It never opens the store or calls the socket/adapter, so it is safe while the
// caller holds a platform transaction lock. Output appears only on success.
func appCanon(in io.Reader, out io.Writer) error {
	const maxConfig = 1 << 20
	b, err := io.ReadAll(io.LimitReader(in, maxConfig+1))
	if err != nil {
		return err
	}
	if len(b) > maxConfig || !utf8.Valid(b) || !json.Valid(b) {
		return fmt.Errorf("invalid or oversized app configuration")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	var cfg rules.AppConfig
	if err := d.Decode(&cfg); err != nil {
		return err
	}
	canonical, err := cfg.Canonical()
	if err != nil {
		return err
	}
	if canonical.Rules != "" {
		_, err = fmt.Fprintln(out, canonical.Rules)
	}
	return err
}
