package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

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
	if err := httpserver.Serve(ctx, c, listener, logger); err != nil {
		logger.Error("server_stopped_with_error")
		os.Exit(1)
	}
}
