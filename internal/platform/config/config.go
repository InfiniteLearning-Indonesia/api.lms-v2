// Package config defines the deliberately restricted bootstrap runtime.
package config

import (
	"encoding/hex"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	Mode              string
	Address           string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	TLSCertFile       string
	TLSKeyFile        string
	Origin            string
	BridgePublicKey   string
}

// Load does not read dotenv files or enable a shared deployment. Until DB,
// identity and ownership gates exist, the server is loopback-synthetic only.
func Load(getenv func(string) string) (Config, error) {
	c := Config{Mode: getenv("LMS_MODE"), Address: getenv("LMS_HTTP_ADDR"), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, ShutdownTimeout: 5 * time.Second}
	if c.Mode != "local-synthetic" && c.Mode != "v3-local-synthetic" {
		return Config{}, errors.New("LMS_MODE must explicitly select an implemented local mode")
	}
	host, port, err := net.SplitHostPort(c.Address)
	if err != nil {
		return Config{}, errors.New("LMS_HTTP_ADDR must be an explicit loopback IP and port")
	}
	ip := net.ParseIP(host)
	p, portErr := strconv.Atoi(port)
	if ip == nil || !ip.IsLoopback() || portErr != nil || p < 0 || p > 65535 {
		return Config{}, errors.New("only loopback IP listeners are permitted")
	}
	if c.Mode == "v3-local-synthetic" {
		if p == 0 {
			return Config{}, errors.New("business runtime requires an explicit port")
		}
		c.TLSCertFile, c.TLSKeyFile = getenv("LMS_TLS_CERT_FILE"), getenv("LMS_TLS_KEY_FILE")
		c.Origin, c.BridgePublicKey = getenv("LMS_ORIGIN"), getenv("LMS_BRIDGE_PUBLIC_KEY")
		if c.Origin != "https://"+c.Address {
			return Config{}, errors.New("LMS_ORIGIN must exactly match the direct TLS listener")
		}
		key, err := hex.DecodeString(c.BridgePublicKey)
		if err != nil || len(key) != 32 {
			return Config{}, errors.New("LMS_BRIDGE_PUBLIC_KEY must be an Ed25519 public key")
		}
		for _, path := range []string{c.TLSCertFile, c.TLSKeyFile} {
			if !filepath.IsAbs(path) || filepath.Clean(path) != path {
				return Config{}, errors.New("TLS files must use absolute clean paths")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 1<<20 {
				return Config{}, errors.New("TLS file rejected")
			}
		}
	}
	return c, nil
}
