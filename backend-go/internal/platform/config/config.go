// Package config defines the deliberately restricted bootstrap runtime.
package config

import (
	"errors"
	"net"
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
}

// Load does not read dotenv files or enable a shared deployment. Until DB,
// identity and ownership gates exist, the server is loopback-synthetic only.
func Load(getenv func(string) string) (Config, error) {
	c := Config{Mode: getenv("LMS_MODE"), Address: getenv("LMS_HTTP_ADDR"), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, ShutdownTimeout: 5 * time.Second}
	if c.Mode != "local-synthetic" {
		return Config{}, errors.New("LMS_MODE must explicitly be local-synthetic; shared runtime is not implemented")
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
	return c, nil
}
