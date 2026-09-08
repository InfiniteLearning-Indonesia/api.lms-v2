// Package dbinspect implements operator-only, read-only PostgreSQL inspection.
// It is deliberately not wired into cmd/api or its readiness handler.
package dbinspect

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type Settings struct {
	host, port, user, password, database, caFile string
}

func (Settings) String() string               { return "[REDACTED_DB_SETTINGS]" }
func (s Settings) GoString() string           { return s.String() }
func (Settings) MarshalJSON() ([]byte, error) { return []byte(`"[REDACTED_DB_SETTINGS]"`), nil }

// Parse reads selected literal values, not shell instructions or interpolation.
// Non-DB integration secrets are ignored, not forwarded to driver/runtime.
func Parse(data []byte) (Settings, error) {
	var empty Settings
	if len(data) > 1024*1024 || strings.ContainsRune(string(data), 0) {
		return empty, errors.New("CONFIG_INVALID")
	}
	allowed := map[string]bool{"DB_HOST": true, "DB_PORT": true, "DB_USER": true, "DB_USERNAME": true, "DB_PASSWORD": true, "DB_NAME": true, "DB_DATABASE": true, "DB_SSL": true, "DB_SSL_ROOT_CERT": true}
	values := map[string]string{}
	for _, line := range strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !allowed[key] {
			continue
		}
		if _, duplicate := values[key]; duplicate {
			return empty, errors.New("CONFIG_DUPLICATE_DB_KEY")
		}
		raw = strings.TrimSpace(raw)
		if strings.HasPrefix(raw, "'") || strings.HasPrefix(raw, `"`) {
			quote := raw[0]
			if len(raw) < 2 || raw[len(raw)-1] != quote {
				return empty, errors.New("CONFIG_INVALID_QUOTING")
			}
			raw = raw[1 : len(raw)-1]
		} else if i := strings.Index(raw, " #"); i >= 0 {
			raw = strings.TrimSpace(raw[:i])
		}
		values[key] = raw
	}
	alias := func(primary, alternate string) (string, error) {
		a, b := values[primary], values[alternate]
		if a != "" && b != "" && a != b {
			return "", errors.New("CONFIG_CONFLICTING_ALIASES")
		}
		if a != "" {
			return a, nil
		}
		return b, nil
	}
	user, err := alias("DB_USER", "DB_USERNAME")
	if err != nil {
		return empty, err
	}
	database, err := alias("DB_NAME", "DB_DATABASE")
	if err != nil {
		return empty, err
	}
	s := Settings{values["DB_HOST"], values["DB_PORT"], user, values["DB_PASSWORD"], database, values["DB_SSL_ROOT_CERT"]}
	if s.host == "" || s.user == "" || s.password == "" || s.database == "" {
		return empty, errors.New("CONFIG_MISSING_DB_FIELDS")
	}
	port, err := strconv.Atoi(s.port)
	if err != nil || port < 1 || port > 65535 {
		return empty, errors.New("CONFIG_INVALID_PORT")
	}
	if strings.ContainsAny(s.host, "/\\@, \t\r\n") || strings.ContainsAny(s.user+s.database, "\r\n") {
		return empty, errors.New("CONFIG_INVALID_IDENTITY")
	}
	if ssl := strings.ToLower(values["DB_SSL"]); ssl != "" && ssl != "true" {
		return empty, errors.New("TLS_REQUIRED")
	}
	return s, nil
}

func Load(path string) (Settings, error) {
	var empty Settings
	if !filepath.IsAbs(path) {
		return empty, errors.New("CONFIG_ABSOLUTE_PATH_REQUIRED")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1024*1024 {
		return empty, errors.New("CONFIG_FILE_UNAVAILABLE")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return empty, errors.New("CONFIG_FILE_UNAVAILABLE")
	}
	s, err := Parse(data)
	if err == nil && s.caFile != "" && !filepath.IsAbs(s.caFile) {
		s.caFile = filepath.Join(filepath.Dir(path), s.caFile)
	}
	return s, err
}

func (s Settings) Fingerprint() string {
	return digest([]byte(strings.Join([]string{strings.ToLower(s.host), s.port, s.user, s.database}, "\x00")))
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func (s Settings) DriverConfig() (*pgx.ConnConfig, error) {
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, errors.New("TLS_SYSTEM_ROOTS_UNAVAILABLE")
	}
	if s.caFile != "" {
		info, err := os.Lstat(s.caFile)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1024*1024 {
			return nil, errors.New("TLS_CA_FILE_UNAVAILABLE")
		}
		pem, err := os.ReadFile(s.caFile)
		if err != nil || !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("TLS_CA_INVALID")
		}
	}
	u := url.URL{Scheme: "postgres", User: url.UserPassword(s.user, s.password), Host: net.JoinHostPort(s.host, s.port), Path: "/" + s.database}
	q := url.Values{"sslmode": {"verify-full"}, "connect_timeout": {"5"}}
	u.RawQuery = q.Encode()
	c, err := pgx.ParseConfig(u.String())
	if err != nil {
		return nil, errors.New("DRIVER_CONFIG_REJECTED")
	}
	// Override environment-derived libpq defaults. Never permit non-TLS fallback,
	// inherited PGOPTIONS/search_path/tracers, or automatic prepared statements.
	c.Host = s.host
	c.Port = uint16(mustPort(s.port))
	c.User = s.user
	c.Password = s.password
	c.Database = s.database
	c.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: s.host, RootCAs: roots}
	c.Fallbacks = nil
	c.ConnectTimeout = 5 * time.Second
	c.RuntimeParams = map[string]string{"application_name": "lmsv3-readonly-inspect", "default_transaction_read_only": "on"}
	c.DefaultQueryExecMode = pgx.QueryExecModeExec
	c.StatementCacheCapacity = 0
	c.DescriptionCacheCapacity = 0
	c.Tracer = nil
	c.DialFunc = (&net.Dialer{Timeout: 5 * time.Second}).DialContext
	return c, nil
}

func mustPort(value string) int { n, _ := strconv.Atoi(value); return n }

// SafeError never exposes pgx's connection string, SQL detail, host or password.
func SafeError(err error) string {
	var certError *tls.CertificateVerificationError
	var unknown x509.UnknownAuthorityError
	var host x509.HostnameError
	if errors.As(err, &certError) || errors.As(err, &unknown) || errors.As(err, &host) {
		return "TLS_CERTIFICATE_VERIFICATION_FAILED"
	}
	var sqlState interface{ SQLState() string }
	if errors.As(err, &sqlState) {
		code := sqlState.SQLState()
		valid := len(code) == 5
		for _, ch := range code {
			if !(ch >= '0' && ch <= '9' || ch >= 'A' && ch <= 'Z') {
				valid = false
			}
		}
		if valid {
			return fmt.Sprintf("POSTGRES_SQLSTATE_%s", code)
		}
	}
	return "CONNECTION_OR_QUERY_FAILED"
}
