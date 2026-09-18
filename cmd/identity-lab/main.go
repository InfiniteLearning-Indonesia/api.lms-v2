// Opt-in loopback TLS lab. No schema/seed/issuer private key at API startup.
package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	attendancedomain "lmsv3/internal/attendance"
	"lmsv3/internal/classes"
	contentdomain "lmsv3/internal/content"
	"lmsv3/internal/egress"
	filedomain "lmsv3/internal/files"
	gradebookdomain "lmsv3/internal/gradebook"
	"lmsv3/internal/identity"
	"lmsv3/internal/labdb"
	logbookdomain "lmsv3/internal/logbook"
	notificationdomain "lmsv3/internal/notifications"
	submissiondomain "lmsv3/internal/submissions"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
	checkpoint := os.Getenv("QA_CHECKPOINT")
	if checkpoint == "cp12" || checkpoint == "m05" || checkpoint == "m06" || checkpoint == "m06cp03" || checkpoint == "m07" || checkpoint == "m07cp02" || checkpoint == "m07cp03" || checkpoint == "m08cp01" || checkpoint == "m08cp02" || checkpoint == "m08cp03" || checkpoint == "m08cp04" || checkpoint == "m15cp01" || checkpoint == "m15cp02" || checkpoint == "m15cp03" || checkpoint == "m15cp04" || checkpoint == "m15cp05" || checkpoint == "m11cp01" || checkpoint == "m12cp01" || checkpoint == "m12cp02" || checkpoint == "m12cp03" || checkpoint == "m13cp01" || checkpoint == "m13cp02" || checkpoint == "m09cp01" || checkpoint == "m09cp02" {
		store = classes.NewContextual(pool)
	} else if checkpoint != "cp11" {
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
	business := classes.Handler(store, logger)
	if checkpoint == "m06" || checkpoint == "m06cp03" || checkpoint == "m08cp02" || checkpoint == "m08cp03" || checkpoint == "m08cp04" {
		objects, objectErr := filedomain.NewLocalStore(os.Getenv("QA_FILE_ROOT"))
		if objectErr != nil {
			os.Exit(1)
		}
		options := []filedomain.Option{}
		if checkpoint == "m06cp03" {
			fetcher, fetchErr := egress.New(egress.Config{AllowedHosts: []string{"localhost"}, MaxBytes: filedomain.MaxBytes, MaxRedirects: 2, Timeout: time.Second, Concurrency: 2})
			if fetchErr != nil {
				os.Exit(1)
			}
			options = append(options, filedomain.WithRemoteFetcher(fetcher))
		}
		fileStore, fileErr := filedomain.New(pool, objects, filedomain.FixtureScanner{}, 2*time.Second, options...)
		if fileErr != nil {
			os.Exit(1)
		}
		classHandler := business
		fileHandler := filedomain.Handler(fileStore, logger)
		business = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if filedomain.OwnsPath(r.URL.Path) {
				fileHandler.ServeHTTP(w, r)
				return
			}
			classHandler.ServeHTTP(w, r)
		})
	}
	if checkpoint == "m07" || checkpoint == "m07cp02" || checkpoint == "m07cp03" || checkpoint == "m08cp01" || checkpoint == "m08cp02" || checkpoint == "m08cp03" || checkpoint == "m08cp04" || checkpoint == "m09cp01" || checkpoint == "m09cp02" {
		classHandler := business
		contentHandler := contentdomain.Handler(contentdomain.New(pool), logger)
		business = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if contentdomain.OwnsPath(r.URL.Path) {
				contentHandler.ServeHTTP(w, r)
				return
			}
			classHandler.ServeHTTP(w, r)
		})
	}
	if checkpoint == "m08cp01" || checkpoint == "m08cp02" || checkpoint == "m08cp03" || checkpoint == "m08cp04" || checkpoint == "m09cp01" || checkpoint == "m09cp02" {
		priorHandler := business
		submissionHandler := submissiondomain.Handler(submissiondomain.New(pool), logger)
		business = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if submissiondomain.OwnsPath(r.URL.Path) {
				submissionHandler.ServeHTTP(w, r)
				return
			}
			priorHandler.ServeHTTP(w, r)
		})
	}
	if checkpoint == "m11cp01" {
		priorHandler := business
		meetingHandler := attendancedomain.Handler(attendancedomain.New(pool), logger)
		business = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if attendancedomain.OwnsPath(r.URL.Path) {
				meetingHandler.ServeHTTP(w, r)
				return
			}
			priorHandler.ServeHTTP(w, r)
		})
	}
	if checkpoint == "m12cp01" || checkpoint == "m12cp02" || checkpoint == "m12cp03" {
		priorHandler := business
		periodHandler := logbookdomain.Handler(logbookdomain.New(pool), logger)
		business = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if logbookdomain.OwnsPath(r.URL.Path) {
				periodHandler.ServeHTTP(w, r)
				return
			}
			priorHandler.ServeHTTP(w, r)
		})
	}
	if checkpoint == "m13cp01" || checkpoint == "m13cp02" {
		priorHandler := business
		notificationHandler := notificationdomain.Handler(notificationdomain.New(pool), logger)
		business = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if notificationdomain.OwnsPath(r.URL.Path) {
				notificationHandler.ServeHTTP(w, r)
				return
			}
			priorHandler.ServeHTTP(w, r)
		})
	}
	if checkpoint == "m09cp01" || checkpoint == "m09cp02" {
		priorHandler := business
		gradebookHandler := gradebookdomain.Handler(gradebookdomain.New(pool), logger)
		business = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if gradebookdomain.OwnsPath(r.URL.Path) {
				gradebookHandler.ServeHTTP(w, r)
				return
			}
			priorHandler.ServeHTTP(w, r)
		})
	}
	server.Config.Handler = identity.Gateway(service, origin, business, logger)
	server.StartTLS()
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"msg": "identity_lab_started", "address": server.URL, "checkpoint": checkpoint})
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
