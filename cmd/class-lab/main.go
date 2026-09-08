// class-lab serves the real business handler only inside the newly provisioned
// loopback QA cluster. No dotenv, login bypass, seed, migration or shared mode.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"lmsv3/internal/classes"
	"lmsv3/internal/labdb"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	// Local harness control channel only; no HTTP shutdown backdoor. Normal
	// shutdown also lets Go emit instrumented API coverage before process exit.
	go func() {
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if line == "shutdown\n" {
			cancel()
		}
	}()
	connect, stop := context.WithTimeout(ctx, 5*time.Second)
	pool, err := labdb.Runtime(connect)
	stop()
	if err != nil {
		os.Stderr.WriteString("LOCAL_RUNTIME_GATE_REJECTED\n")
		os.Exit(1)
	}
	defer pool.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(1)
	}
	server := &http.Server{Handler: classes.Handler(classes.New(pool), slog.New(slog.NewJSONHandler(os.Stdout, nil))), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 7 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"msg": "class_lab_started", "address": listener.Addr().String(), "mode": "local-synthetic"})
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		shutdown, finish := context.WithTimeout(context.Background(), 5*time.Second)
		defer finish()
		if server.Shutdown(shutdown) != nil {
			_ = server.Close()
		}
		<-done
	case <-done:
	}
}
