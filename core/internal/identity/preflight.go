package identity

import "fmt"

// CheckStorage checks existing versioned identity documents without importing,
// expiring, caching or writing anything. Startup calls it before native recovery.
// Missing files are left to their ordinary first-use/import paths.
func CheckStorage(st Storage) error {
	var devices devicesDoc
	found, err := st.ReadDocument(devicesFile, &devices)
	if err != nil {
		return fmt.Errorf("devices: %w", err)
	}
	if found && devices.Version != schemaVersion {
		return fmt.Errorf("devices: unsupported schema version %d", devices.Version)
	}

	var ledger ledgerDoc
	found, err = st.ReadDocument(requestsFile, &ledger)
	if err != nil {
		return fmt.Errorf("requests: %w", err)
	}
	if found {
		if err := validateLedger(ledger); err != nil {
			return err
		}
	}

	var apps newAppsDoc
	found, err = st.ReadDocument(newAppsFile, &apps)
	if err != nil {
		return fmt.Errorf("newapps: %w", err)
	}
	if found {
		if apps.Version != newAppsVersion {
			return fmt.Errorf("newapps: unsupported schema version %d", apps.Version)
		}
		if err := validateNewApps(apps.Profiles); err != nil {
			return err
		}
	}
	return nil
}
