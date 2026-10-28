package configuration

import (
	"encoding/json"
	"errors"
)

// OwnershipDocument is durable evidence that the household selected the core.
// Native adapters use its presence only to refuse legacy writers. The core alone
// validates its contents. It is not a revision/usage anti-rollback counter.
const OwnershipDocument = "authority.json"

// OwnershipFormat means the native adapter also refuses legacy writers when the
// ownership document (or either document's presence witness) remains while
// the household configuration is unavailable.
const OwnershipFormat = "authority-v2"

type ownership struct {
	Version int    `json:"version"`
	Owner   string `json:"owner"`
}

func (a *Authority) ownership() (bool, error) {
	var raw json.RawMessage
	found, err := a.store.ReadDocument(OwnershipDocument, &raw)
	if err != nil || !found {
		return found, err
	}
	var o ownership
	if err := decode(raw, &o); err != nil {
		return true, err
	}
	if o.Version != 1 || o.Owner != "core" {
		return true, errors.New("unsupported authority document")
	}
	return true, nil
}

// The first configuration journal is persisted/read back before this floor;
// both must be durable before any native apply. A lost floor write leaves the
// journal as the authority, and restart completes adoption before applying it.
func (a *Authority) ensureOwnership() error {
	found, err := a.ownership()
	if err != nil || found {
		return err
	}
	want := ownership{Version: 1, Owner: "core"}
	if err := a.store.WriteDocument(OwnershipDocument, want); err != nil {
		return err
	}
	found, err = a.ownership()
	if err != nil || !found {
		return errors.New("authority read-back failed")
	}
	return nil
}
