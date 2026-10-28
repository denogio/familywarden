package configuration

import "fmt"

// CheckStorage checks existing configuration and DNS documents without recovery,
// native calls or writes. Owned households must already have an identity registry;
// the production composition uses the returned ownership to require it.
func CheckStorage(st Storage) (owned bool, err error) {
	a := &Authority{store: st}
	_, owned, err = a.load()
	if err != nil {
		return owned, fmt.Errorf("configuration: %w", err)
	}
	var dns dnsState
	found, err := st.ReadDocument(dnsDocument, &dns)
	if err != nil {
		return owned, fmt.Errorf("dns: %w", err)
	}
	if found {
		return owned, validateDNSState(dns)
	}
	return owned, nil
}
