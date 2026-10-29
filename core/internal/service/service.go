package service

import (
	"net/http"

	"github.com/denogio/familywarden/core/internal/configuration"
	"github.com/denogio/familywarden/core/internal/identity"
	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/store"
)

// Service is the production composition, independent of a particular router.
// The native host must serialize writer lifetimes before calling New: preflight
// is read-only validation, not a cross-file transaction against concurrent writers.
// The executable supplies its adapter and storage; tests use the same constructor.
type Service struct {
	Configuration *configuration.Authority
	Identity      *identity.Service
	Handler       http.Handler
}

func New(ad platform.Adapter, backend configuration.Backend, st *store.Store, version string) (*Service, error) {
	// Reject incompatible stored state before either constructor can recover a
	// configuration, apply native rules or import/remove legacy identities.
	if err := st.CheckPresence(); err != nil {
		return nil, err
	}
	if err := st.CheckGeneration(); err != nil {
		return nil, err
	}
	owned, err := configuration.CheckStorage(st)
	if err != nil {
		return nil, err
	}
	if err := identity.CheckStorage(st, owned); err != nil {
		return nil, err
	}
	if err := st.AdoptPresence(); err != nil {
		return nil, err
	}
	cfg, err := configuration.New(ad, backend, st)
	if err != nil {
		return nil, err
	}
	id, err := identity.New(cfg, st)
	if err != nil {
		cfg.Close()
		return nil, err
	}
	id.Usage = st
	id.Inventory = st
	id.Generation = st.Generation
	return &Service{Configuration: cfg, Identity: id, Handler: WithConfiguration(Handler(id, version), cfg)}, nil
}

func (s *Service) Close() { s.Configuration.Close() }
