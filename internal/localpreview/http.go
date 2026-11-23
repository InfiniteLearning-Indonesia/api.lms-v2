package localpreview

import (
	"encoding/hex"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/aisuggestions"
	"lmsv3/internal/attendance"
	"lmsv3/internal/classes"
	"lmsv3/internal/completion"
	"lmsv3/internal/content"
	"lmsv3/internal/files"
	"lmsv3/internal/gradebook"
	"lmsv3/internal/identity"
	"lmsv3/internal/logbook"
	"lmsv3/internal/notifications"
	"lmsv3/internal/reporting"
	"lmsv3/internal/submissions"
)

func Handler(c Config, origin string, pool *pgxpool.Pool, env func(string) string, logger *slog.Logger) (http.Handler, func(), error) {
	lifecycle := &identity.Lifecycle{Pool: pool}
	passwords, err := identity.NewPasswords(lifecycle, c.DeliveryKey)
	if err != nil {
		return nil, nil, err
	}
	objects, err := files.NewLocalStore(filepath.Join(c.Root, "files"))
	if err != nil {
		return nil, nil, err
	}
	fileStore, err := files.New(pool, objects, files.FixtureScanner{}, time.Minute)
	if err != nil {
		return nil, nil, err
	}
	domain := classes.Handler(classes.NewWithAccountInvitations(pool, passwords), logger)
	completionStore := completion.New(pool)
	routes := []struct {
		owns    func(string) bool
		handler http.Handler
	}{
		{files.OwnsPath, files.Handler(fileStore, logger)},
		{submissions.OwnsPath, submissions.Handler(submissions.New(pool), logger)},
		{completion.CredentialOwnsPath, completion.CredentialHandler(completionStore, logger)},
		{completion.OwnsPath, completion.Handler(completionStore, logger)},
		{gradebook.OwnsPath, gradebook.Handler(gradebook.New(pool), logger)},
		{attendance.OwnsPath, attendance.Handler(attendance.New(pool), logger)},
		{logbook.OwnsPath, logbook.Handler(logbook.New(pool), logger)},
		{notifications.OwnsPath, notifications.Handler(notifications.New(pool), logger)},
		{aisuggestions.OwnsPath, aisuggestions.Handler(aisuggestions.New(pool, aisuggestions.RehearsalProvider{}), logger)},
		{reporting.OwnsPath, reporting.Handler(reporting.Store{Pool: pool}, logger)},
		{content.OwnsPath, content.Handler(content.New(pool), logger)},
	}
	business := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, route := range routes {
			if route.owns(r.URL.Path) {
				route.handler.ServeHTTP(w, r)
				return
			}
		}
		domain.ServeHTTP(w, r)
	})
	api := identity.GatewayWithPasswords(lifecycle, passwords, origin, business, logger)
	client, secret := env("LMS_PREVIEW_GOOGLE_CLIENT_ID"), env("LMS_PREVIEW_GOOGLE_CLIENT_SECRET")
	if client != "" || secret != "" {
		key, err := hex.DecodeString(env("LMS_PREVIEW_GOOGLE_FLOW_KEY"))
		if err != nil {
			return nil, nil, errTarget
		}
		google, err := identity.NewGoogle(passwords, identity.GoogleConfig{Origin: origin, ClientID: client, ClientSecret: secret, FlowKey: key})
		if err != nil {
			return nil, nil, err
		}
		api, err = identity.GatewayWithGoogle(google, origin, business, logger)
		if err != nil {
			return nil, nil, err
		}
	}
	if c.WebRoot == "" {
		return api, func() {}, nil
	}
	if !filepath.IsAbs(c.WebRoot) || filepath.Clean(c.WebRoot) != c.WebRoot {
		return nil, nil, errTarget
	}
	root, err := os.OpenRoot(c.WebRoot)
	if err != nil {
		return nil, nil, err
	}
	info, err := fs.Stat(root.FS(), "index.html")
	if err != nil || !info.Mode().IsRegular() {
		_ = root.Close()
		return nil, nil, errTarget
	}
	return withWeb(api, root.FS()), func() { _ = root.Close() }, nil
}

func withWeb(api http.Handler, web fs.FS) http.Handler {
	static := http.FileServerFS(web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean(r.URL.Path)
		if p == "/v3" || strings.HasPrefix(p, "/v3/") || p == "/health" || strings.HasPrefix(p, "/health/") {
			api.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		if p == "/" || p == "/auth/google/callback" || p == "/login" {
			page := r.Clone(r.Context())
			page.URL.Path, page.URL.RawPath, page.URL.RawQuery = "/", "", ""
			static.ServeHTTP(w, page)
			return
		}
		// Mount built assets and the two public SVG paths emitted by fe-test.
		publicSVG := p == "/favicon.svg" || p == "/icons.svg"
		if !publicSVG && (!strings.HasPrefix(p, "/assets/") || strings.HasSuffix(p, ".map")) {
			http.NotFound(w, r)
			return
		}
		for _, part := range strings.Split(strings.TrimPrefix(p, "/"), "/") {
			if strings.HasPrefix(part, ".") || strings.ContainsAny(part, "\\:") {
				http.NotFound(w, r)
				return
			}
		}
		info, err := fs.Stat(web, strings.TrimPrefix(p, "/"))
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		asset := r.Clone(r.Context())
		asset.URL.Path, asset.URL.RawPath = p, ""
		static.ServeHTTP(w, asset)
	})
}
