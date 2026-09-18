package egress

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ErrPolicy = errors.New("EGRESS_POLICY_REJECTED")
var ErrAddress = errors.New("EGRESS_ADDRESS_REJECTED")
var ErrTooLarge = errors.New("EGRESS_RESPONSE_TOO_LARGE")
var ErrTimeout = errors.New("EGRESS_TIMEOUT")
var ErrUnavailable = errors.New("EGRESS_UNAVAILABLE")

var hostPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?$`)

type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type Config struct {
	AllowedHosts []string
	MaxBytes     int64
	MaxRedirects int
	Timeout      time.Duration
	Concurrency  int
	Resolver     Resolver
	// Transport is only for deterministic adapters/tests. Production should
	// leave it nil so the validated dial path below remains authoritative.
	Transport http.RoundTripper
}

type Result struct {
	Data      []byte
	MediaType string
}

type Fetcher struct {
	client   *http.Client
	resolver Resolver
	allowed  map[string]struct{}
	maxBytes int64
	timeout  time.Duration
	sem      chan struct{}
}

func New(config Config) (*Fetcher, error) {
	if config.MaxBytes < 1 || config.MaxBytes > 8<<20 || config.MaxRedirects < 0 || config.MaxRedirects > 5 || config.Timeout < 100*time.Millisecond || config.Timeout > 10*time.Second || config.Concurrency < 1 || config.Concurrency > 32 {
		return nil, ErrPolicy
	}
	allowed := make(map[string]struct{}, len(config.AllowedHosts))
	for _, candidate := range config.AllowedHosts {
		host := normalizeHost(candidate)
		if host == "" || net.ParseIP(host) != nil || !hostPattern.MatchString(host) || strings.Contains(host, "..") {
			return nil, ErrPolicy
		}
		allowed[host] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil, ErrPolicy
	}
	resolver := config.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	fetcher := &Fetcher{resolver: resolver, allowed: allowed, maxBytes: config.MaxBytes, timeout: config.Timeout, sem: make(chan struct{}, config.Concurrency)}
	base := config.Transport
	if base == nil {
		dialer := &net.Dialer{Timeout: config.Timeout, KeepAlive: 30 * time.Second}
		base = &http.Transport{
			Proxy:                 nil,
			DisableCompression:    true,
			ForceAttemptHTTP2:     true,
			TLSHandshakeTimeout:   config.Timeout,
			ResponseHeaderTimeout: config.Timeout,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(address)
				if err != nil || port != "443" {
					return nil, ErrPolicy
				}
				addresses, err := fetcher.resolve(ctx, host)
				if err != nil {
					return nil, err
				}
				// Connect to an address that was validated in this exact dial call;
				// never resolve the hostname again in the OS dialer.
				return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].String(), port))
			},
		}
	}
	fetcher.client = &http.Client{
		Transport: validatingTransport{fetcher: fetcher, next: base},
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) > config.MaxRedirects {
				return ErrPolicy
			}
			return nil
		},
	}
	return fetcher, nil
}

type validatingTransport struct {
	fetcher *Fetcher
	next    http.RoundTripper
}

func (t validatingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := t.fetcher.validateURL(request.Context(), request.URL); err != nil {
		return nil, err
	}
	clone := request.Clone(request.Context())
	clone.Header = make(http.Header)
	clone.Header.Set("Accept", "application/pdf,image/png,image/jpeg,text/plain")
	clone.Header.Set("User-Agent", "lms-v3-fetch/1")
	return t.next.RoundTrip(clone)
}

func normalizeHost(value string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
}

func (f *Fetcher) validateURL(ctx context.Context, target *url.URL) error {
	if target == nil || target.Scheme != "https" || target.User != nil || target.Fragment != "" || target.Hostname() == "" {
		return ErrPolicy
	}
	port := target.Port()
	if port != "" && port != "443" {
		return ErrPolicy
	}
	_, err := f.resolve(ctx, target.Hostname())
	return err
}

func (f *Fetcher) resolve(ctx context.Context, rawHost string) ([]netip.Addr, error) {
	host := normalizeHost(rawHost)
	if _, ok := f.allowed[host]; !ok || net.ParseIP(host) != nil {
		return nil, ErrPolicy
	}
	resolved, err := f.resolver.LookupIPAddr(ctx, host)
	if err != nil || len(resolved) == 0 {
		return nil, ErrUnavailable
	}
	addresses := make([]netip.Addr, 0, len(resolved))
	for _, item := range resolved {
		address, ok := netip.AddrFromSlice(item.IP)
		if !ok || blocked(address.Unmap()) {
			return nil, ErrAddress
		}
		addresses = append(addresses, address.Unmap())
	}
	return addresses, nil
}

var forbiddenPrefixes = func() []netip.Prefix {
	values := []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24",
		"198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
		"100::/64", "2001:db8::/32",
	}
	result := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		result = append(result, netip.MustParsePrefix(value))
	}
	return result
}()

func blocked(address netip.Addr) bool {
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() || address.IsUnspecified() {
		return true
	}
	for _, prefix := range forbiddenPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (Result, error) {
	select {
	case f.sem <- struct{}{}:
		defer func() { <-f.sem }()
	case <-ctx.Done():
		return Result{}, ErrTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	target, err := url.Parse(rawURL)
	if err != nil || target.String() != rawURL {
		return Result{}, ErrPolicy
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return Result{}, ErrPolicy
	}
	response, err := f.client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Result{}, ErrTimeout
		}
		if errors.Is(err, ErrPolicy) || errors.Is(err, ErrAddress) {
			return Result{}, err
		}
		return Result{}, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 || response.Header.Get("Content-Encoding") != "" {
		return Result{}, ErrUnavailable
	}
	if length := response.Header.Get("Content-Length"); length != "" {
		parsed, parseErr := strconv.ParseInt(length, 10, 64)
		if parseErr != nil || parsed < 0 {
			return Result{}, ErrUnavailable
		}
		if parsed > f.maxBytes {
			return Result{}, ErrTooLarge
		}
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil {
		return Result{}, ErrUnavailable
	}
	allowedMedia := map[string]bool{"application/pdf": true, "image/png": true, "image/jpeg": true, "text/plain": true}
	if !allowedMedia[strings.ToLower(mediaType)] {
		return Result{}, ErrPolicy
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, f.maxBytes+1))
	if err != nil {
		return Result{}, ErrUnavailable
	}
	if int64(len(data)) > f.maxBytes {
		return Result{}, ErrTooLarge
	}
	if len(data) == 0 {
		return Result{}, ErrUnavailable
	}
	return Result{Data: data, MediaType: strings.ToLower(mediaType)}, nil
}

func (f *Fetcher) String() string {
	return fmt.Sprintf("bounded-egress(max_bytes=%d)", f.maxBytes)
}
