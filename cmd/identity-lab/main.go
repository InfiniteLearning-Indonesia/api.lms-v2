// Opt-in loopback TLS lab. No schema/seed/issuer private key at API startup.
package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"lmsv3/internal/classes"
	"lmsv3/internal/identity"
	"lmsv3/internal/labdb"
	"log/slog"
	"net/http/httptest"
	"os"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	pool, err := labdb.Runtime(ctx)
	if err != nil {
		os.Exit(1)
	}
	defer pool.Close()
	writer, err := labdb.IdentityRuntime(ctx)
	cancel()
	if err != nil {
		os.Exit(1)
	}
	defer writer.Close()
	key, err := hex.DecodeString(os.Getenv("QA_BRIDGE_PUBLIC"))
	if err != nil || len(key) != ed25519.PublicKeySize {
		os.Exit(1)
	}
	service := &identity.Lifecycle{Pool: writer, Verifier: identity.Verifier{Issuer: "https://synthetic-issuer.invalid", Audience: "lms-v3-local", Keys: map[string]ed25519.PublicKey{"fixture-1": key}}}
	store := classes.NewBridge(pool)
	if os.Getenv("QA_CHECKPOINT") == "cp12" {
		store = classes.NewContextual(pool)
	} else if os.Getenv("QA_CHECKPOINT") != "cp11" {
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	server := httptest.NewUnstartedServer(nil)
	server.Config.ReadHeaderTimeout = 3 * time.Second
	server.Config.ReadTimeout = 5 * time.Second
	server.Config.WriteTimeout = 7 * time.Second
	server.Config.IdleTimeout = 30 * time.Second
	server.Config.MaxHeaderBytes = 16 << 10
	origin := "https://" + server.Listener.Addr().String()
	server.Config.Handler = identity.Gateway(service, origin, classes.Handler(store, logger), logger)
	server.StartTLS()
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"msg": "identity_lab_started", "address": server.URL, "checkpoint": os.Getenv("QA_CHECKPOINT")})
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if line != "shutdown\n" {
		server.Close()
		os.Exit(1)
	}
	shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if server.Config.Shutdown(shutdown) != nil {
		server.CloseClientConnections()
	}
	server.Close()
}
