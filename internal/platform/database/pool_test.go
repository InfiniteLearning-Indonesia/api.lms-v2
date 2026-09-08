package database

import (
	"crypto/tls"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestRuntimePoolConfig(t *testing.T) {
	c, err := pgx.ParseConfig("postgres://qa_runtime@127.0.0.1:5544/qa_cp05?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	c.Fallbacks = nil
	if Validate(c, false) == nil || Validate(c, true) != nil {
		t.Fatal("TLS/local boundary failed")
	}
	for _, field := range []string{"host", "user", "database"} {
		bad := c.Copy()
		switch field {
		case "host":
			bad.Host = "external.invalid"
		case "user":
			bad.User = "owner"
		case "database":
			bad.Database = "shared"
		}
		if Validate(bad, true) == nil {
			t.Fatal("arbitrary synthetic target accepted")
		}
	}
	c.TLSConfig = &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}
	if Validate(c, false) != nil {
		t.Fatal("verified TLS shape rejected")
	}
	c.TLSConfig.InsecureSkipVerify = true
	if Validate(c, false) == nil {
		t.Fatal("insecure TLS accepted")
	}
}
