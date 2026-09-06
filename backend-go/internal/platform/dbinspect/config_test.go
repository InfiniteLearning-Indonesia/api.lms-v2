package dbinspect

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const syntheticEnv = "DB_HOST=example.invalid\nDB_PORT=5432\nDB_USER=qa\nDB_USERNAME=qa\nDB_NAME=synthetic\nDB_PASSWORD='DO_NOT_EMIT$@#'\nDB_SSL=true\nGOOGLE_CLIENT_SECRET=DO_NOT_EMIT\n"

func TestDatabaseConfigLiteralParsingAndRedaction(t *testing.T) {
	s, err := Parse([]byte(syntheticEnv))
	if err != nil {
		t.Fatal(err)
	}
	if s.password != "DO_NOT_EMIT$@#" || len(s.Fingerprint()) != 64 {
		t.Fatal("literal credential parsing failed")
	}
	data, _ := json.Marshal(s)
	for _, text := range []string{string(data), fmt.Sprintf("%v", s), fmt.Sprintf("%+v", s), fmt.Sprintf("%#v", s), s.Fingerprint()} {
		if strings.Contains(text, "DO_NOT_EMIT") || strings.Contains(text, "example.invalid") {
			t.Fatal("config formatting leaked values")
		}
	}
	other, err := Parse([]byte(strings.ReplaceAll(syntheticEnv, "DO_NOT_EMIT$@#", "different")))
	if err != nil || other.Fingerprint() != s.Fingerprint() {
		t.Fatal("identity fingerprint must not hash password")
	}
	t.Setenv("PGHOST", "untrusted.invalid")
	t.Setenv("PGPASSWORD", "untrusted")
	t.Setenv("PGOPTIONS", "-c search_path=unsafe")
	c, err := s.DriverConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != s.host || c.Password != s.password || c.Database != s.database || c.TLSConfig == nil || c.TLSConfig.InsecureSkipVerify || len(c.Fallbacks) != 0 {
		t.Fatal("unsafe driver override/fallback")
	}
	if c.RuntimeParams["default_transaction_read_only"] != "on" || len(c.RuntimeParams) != 2 || c.DefaultQueryExecMode != pgx.QueryExecModeExec || c.StatementCacheCapacity != 0 || c.ConnectTimeout != 5*time.Second {
		t.Fatal("unbounded or inherited driver behavior")
	}
}

func TestDatabaseConfigRejectsAmbiguityAndUnsafeFiles(t *testing.T) {
	for _, raw := range []string{
		syntheticEnv + "DB_HOST=other.invalid\n",
		strings.Replace(syntheticEnv, "DB_USERNAME=qa", "DB_USERNAME=other", 1),
		strings.Replace(syntheticEnv, "DB_PORT=5432", "DB_PORT=0", 1),
		strings.Replace(syntheticEnv, "DB_SSL=true", "DB_SSL=false", 1),
		strings.Replace(syntheticEnv, "DB_NAME=synthetic", "DB_NAME=", 1),
		strings.Replace(syntheticEnv, "'DO_NOT_EMIT$@#'", "'unclosed", 1),
		syntheticEnv + "\x00",
	} {
		if _, err := Parse([]byte(raw)); err == nil {
			t.Fatal("unsafe configuration accepted")
		}
	}
	if _, err := Load("relative.env"); err == nil {
		t.Fatal("relative config accepted")
	}
	dir := t.TempDir()
	if _, err := Load(dir); err == nil {
		t.Fatal("non-file config accepted")
	}
	path := filepath.Join(dir, "synthetic.env")
	if err := os.WriteFile(path, []byte(syntheticEnv+"DB_SSL_ROOT_CERT=missing.pem\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(s.caFile) {
		t.Fatal("CA path must resolve relative to selected config")
	}
	if _, err = s.DriverConfig(); err == nil {
		t.Fatal("missing CA accepted")
	}
	if err = os.WriteFile(s.caFile, []byte("not a certificate"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DriverConfig(); err == nil {
		t.Fatal("invalid CA accepted")
	}
}

func TestDatabaseTLSVerifiesTrustAndHostname(t *testing.T) {
	server := httptest.NewTLSServer(nil)
	defer server.Close()
	s, _ := Parse([]byte(syntheticEnv))
	cert := server.Certificate()
	if len(cert.DNSNames) == 0 {
		t.Fatal("synthetic TLS fixture requires DNS SAN")
	}
	s.host = cert.DNSNames[0]
	dialer := &net.Dialer{Timeout: time.Second}
	untrusted, err := s.DriverConfig()
	if err != nil {
		t.Fatal(err)
	}
	if conn, err := tls.DialWithDialer(dialer, "tcp", server.Listener.Addr().String(), untrusted.TLSConfig); err == nil {
		conn.Close()
		t.Fatal("untrusted CA accepted")
	}
	s.caFile = filepath.Join(t.TempDir(), "synthetic-ca.pem")
	if err = os.WriteFile(s.caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	trusted, err := s.DriverConfig()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := tls.DialWithDialer(dialer, "tcp", server.Listener.Addr().String(), trusted.TLSConfig)
	if err != nil {
		t.Fatal(err)
	}
	if len(conn.ConnectionState().VerifiedChains) == 0 {
		t.Fatal("no verified chain")
	}
	conn.Close()
	wrong := trusted.TLSConfig.Clone()
	wrong.ServerName = "wrong.invalid"
	if conn, err := tls.DialWithDialer(dialer, "tcp", server.Listener.Addr().String(), wrong); err == nil {
		conn.Close()
		t.Fatal("wrong hostname accepted")
	}
}

func TestDatabaseErrorsAndCatalogAreSafe(t *testing.T) {
	for _, err := range []error{errors.New("DO_NOT_EMIT"), &pgconn.PgError{Code: "28P01", Detail: "DO_NOT_EMIT"}, x509.UnknownAuthorityError{Cert: &x509.Certificate{}}} {
		if strings.Contains(SafeError(err), "DO_NOT_EMIT") {
			t.Fatal("driver error leaked details")
		}
	}
	if SafeError(x509.UnknownAuthorityError{Cert: &x509.Certificate{}}) != "TLS_CERTIFICATE_VERIFICATION_FAILED" {
		t.Fatal("TLS failure not classified")
	}
	for _, sql := range []string{identitySQL, catalogSQL, statisticsSQL, extensionsSQL, dependenciesSQL} {
		if !strings.HasPrefix(sql, "SELECT ") || strings.Contains(sql, ";") || strings.Contains(sql, "FROM public.") {
			t.Fatal("catalog query escaped static read-only scope")
		}
	}
}
