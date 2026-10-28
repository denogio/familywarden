package service

import (
	"net/http"

	"github.com/denogio/familywarden/core/internal/configuration"
	"github.com/denogio/familywarden/core/internal/identity"
	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/store"
)

// Service is the production composition, independent of a particular router.
// The executable supplies its adapter and storage; tests use the same constructor.
type Service struct {
	Configuration *configuration.Authority
	Identity      *identity.Service
	Handler       http.Handler
}

func New(ad platform.Adapter, backend configuration.Backend, st *store.Store, version string) (*Service, error) {
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
	return &Service{Configuration: cfg, Identity: id, Handler: WithConfiguration(Handler(id, version), cfg)}, nil
}

func (s *Service) Close() { s.Configuration.Close() }
