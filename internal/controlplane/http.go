// Package controlplane contains the HTTP and gRPC base servers and the xDS gRPC implementation for envoy.
package controlplane

import (
	"fmt"
	"net/http"
	"time"

	"github.com/CAFxX/httpcompression"
	gorillahandlers "github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"github.com/pomerium/pomerium/config"
	"github.com/pomerium/pomerium/internal/handlers"
	"github.com/pomerium/pomerium/internal/log"
	"github.com/pomerium/pomerium/internal/telemetry"
	"github.com/pomerium/pomerium/internal/telemetry/requestid"
)

func (srv *Server) addHTTPMiddleware(root *mux.Router, cfg *config.Config) {
	compressor, err := httpcompression.DefaultAdapter()
	if err != nil {
		panic(err)
	}

	root.Use(compressor)
	root.Use(srv.reproxy.Middleware)
	root.Use(requestid.HTTPMiddleware())
	root.Use(log.NewHandler(log.Logger))
	root.Use(log.AccessHandler(func(r *http.Request, status, size int, duration time.Duration) {
		log.FromRequest(r).Debug().
			Dur("duration", duration).
			Int("size", size).
			Int("status", status).
			Str("method", r.Method).
			Str("host", r.Host).
			Str("path", r.URL.String()).
			Msg("http-request")
	}))
	root.Use(gorillahandlers.RecoveryHandler())
	root.Use(log.RemoteAddrHandler("ip"))
	root.Use(log.UserAgentHandler("user_agent"))
	root.Use(log.RefererHandler("referer"))
	root.Use(log.RequestIDHandler("request-id"))
	root.Use(telemetry.HTTPStatsHandler(func() string {
		return srv.currentConfig.Load().Options.InstallationID
	}, srv.name))
}

func (srv *Server) mountCommonEndpoints(root *mux.Router, cfg *config.Config) error {
	authenticateURL, err := cfg.Options.GetAuthenticateURL()
	if err != nil {
		return fmt.Errorf("invalid authenticate URL: %w", err)
	}

	rawSigningKey, err := cfg.Options.GetSigningKey()
	if err != nil {
		return fmt.Errorf("invalid signing key: %w", err)
	}

	hpkePrivateKey, err := cfg.Options.GetHPKEPrivateKey()
	if err != nil {
		return fmt.Errorf("invalid hpke private key: %w", err)
	}
	hpkePublicKey := hpkePrivateKey.PublicKey()

	root.HandleFunc("/healthz", handlers.HealthCheck)
	root.HandleFunc("/ping", handlers.HealthCheck)
	root.Handle("/.well-known/pomerium", handlers.WellKnownPomerium(authenticateURL))
	root.Handle("/.well-known/pomerium/", handlers.WellKnownPomerium(authenticateURL))
	root.Path("/.well-known/pomerium/jwks.json").Methods(http.MethodGet).Handler(handlers.JWKSHandler(rawSigningKey, hpkePublicKey))
	return nil
}
