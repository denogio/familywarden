package configuration

import "fmt"

// CheckStorage checks existing configuration and DNS documents without recovery,
// native calls or writes. In particular, checking DNS must not import native state.
func CheckStorage(st Storage) error {
	a := &Authority{store: st}
	if _, _, err := a.load(); err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	var dns dnsState
	found, err := st.ReadDocument(dnsDocument, &dns)
	if err != nil {
		return fmt.Errorf("dns: %w", err)
	}
	if found {
		return validateDNSState(dns)
	}
	return nil
}
