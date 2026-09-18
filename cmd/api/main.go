package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"lmsv3/internal/classes"
	"lmsv3/internal/identity"
	"lmsv3/internal/labdb"
	"lmsv3/internal/migration/runner"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lmsv3/internal/platform/config"
	"lmsv3/internal/platform/httpserver"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c, err := config.Load(os.Getenv)
	if err != nil {
		logger.Error("configuration_rejected")
		os.Exit(2)
	}
	listener, err := net.Listen("tcp", c.Address)
	if err != nil {
		logger.Error("listener_unavailable")
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	logger.Info("local_server_started", "address", listener.Addr().String(), "mode", c.Mode)
	if c.Mode == "local-synthetic" {
		if err := httpserver.Serve(ctx, c, listener, logger); err != nil {
			logger.Error("server_stopped_with_error")
			os.Exit(1)
		}
		return
	}
	startup, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	pool, err := labdb.Runtime(startup)
	if err != nil {
		logger.Error("class_database_rejected")
		os.Exit(1)
	}
	defer pool.Close()
	identityPool, err := labdb.IdentityRuntime(startup)
	if err != nil {
		logger.Error("identity_database_rejected")
		os.Exit(1)
	}
	defer identityPool.Close()
	key, err := hex.DecodeString(c.BridgePublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		logger.Error("identity_key_rejected")
		os.Exit(1)
	}
	service := &identity.Lifecycle{Pool: identityPool, Verifier: identity.Verifier{Issuer: "https://synthetic-issuer.invalid", Audience: "lms-v3-local", Keys: map[string]ed25519.PublicKey{"fixture-1": key}}}
	business := identity.Gateway(service, c.Origin, classes.Handler(classes.NewContextual(pool), logger), logger)
	catalogue, err := labdb.SyntheticCatalogueScope(os.Getenv("QA_CATALOGUE_SCOPE"))
	if err != nil {
		logger.Error("catalogue_scope_rejected")
		os.Exit(1)
	}
	ready := func(check context.Context) error {
		if err := pool.Ping(check); err != nil {
			return err
		}
		if err := identityPool.Ping(check); err != nil {
			return err
		}
		if err := runner.Ready(check, pool, catalogue); err != nil {
			return err
		}
		var ok bool
		if err := identityPool.QueryRow(check, "SELECT EXISTS(SELECT 1 FROM v3identity.site_roles WHERE admin)").Scan(&ok); err != nil || !ok {
			return errors.New("IDENTITY_AUTHORITY_NOT_READY")
		}
		return nil
	}
	handler := httpserver.ApplicationHandler(logger, ready, business)
	if err := httpserver.ServeApplicationTLS(ctx, c, listener, handler, logger); err != nil {
		logger.Error("server_stopped_with_error")
		os.Exit(1)
	}
}
