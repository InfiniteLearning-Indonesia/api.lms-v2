package egress

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type resolverMap map[string][]string

func (r resolverMap) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	values, ok := r[host]
	if !ok {
		return nil, errors.New("unknown fixture host")
	}
	result := make([]net.IPAddr, 0, len(values))
	for _, value := range values {
		result = append(result, net.IPAddr{IP: net.ParseIP(value)})
	}
	return result, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func response(request *http.Request, status int, mediaType, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{mediaType}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}
}

func config(transport http.RoundTripper) Config {
	return Config{AllowedHosts: []string{"files.example.com", "redirect.example.com"}, MaxBytes: 16, MaxRedirects: 2, Timeout: 150 * time.Millisecond, Concurrency: 1, Resolver: resolverMap{
		"files.example.com": {"93.184.216.34"}, "redirect.example.com": {"93.184.216.34"},
	}, Transport: transport}
}

func TestPolicyRejectsPrivateArbitraryAndRedirectTargets(t *testing.T) {
	var calls atomic.Int32
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.URL.Hostname() == "redirect.example.com" {
			value := response(request, http.StatusFound, "text/plain", "redirect")
			value.Header.Set("Location", "https://files.example.com/private.txt")
			return value, nil
		}
		return response(request, http.StatusOK, "text/plain", "safe"), nil
	})
	private := config(transport)
	private.Resolver = resolverMap{"files.example.com": {"127.0.0.1"}, "redirect.example.com": {"93.184.216.34"}}
	fetcher, err := New(private)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fetcher.Fetch(context.Background(), "https://files.example.com/a"); !errors.Is(err, ErrAddress) || calls.Load() != 0 {
		t.Fatal("private target reached transport", err, calls.Load())
	}
	if _, err = fetcher.Fetch(context.Background(), "https://arbitrary-ai.example/a"); !errors.Is(err, ErrPolicy) || calls.Load() != 0 {
		t.Fatal("arbitrary host reached transport", err, calls.Load())
	}
	redirect := config(transport)
	redirect.Resolver = resolverMap{"redirect.example.com": {"93.184.216.34"}, "files.example.com": {"169.254.169.254"}}
	fetcher, _ = New(redirect)
	if _, err = fetcher.Fetch(context.Background(), "https://redirect.example.com/a"); !errors.Is(err, ErrAddress) || calls.Load() != 1 {
		t.Fatal("redirect target was not revalidated", err, calls.Load())
	}
	for _, raw := range []string{"http://files.example.com/a", "https://user@files.example.com/a", "https://files.example.com:8443/a", "https://93.184.216.34/a"} {
		if _, err = fetcher.Fetch(context.Background(), raw); !errors.Is(err, ErrPolicy) {
			t.Fatal("unsafe URL accepted", raw, err)
		}
	}
}

func TestFetchBoundsBytesTimeoutConcurrencyAndHeaders(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "" || request.Header.Get("Cookie") != "" || request.Header.Get("User-Agent") != "lms-v3-fetch/1" {
			t.Fatal("unsafe outbound headers")
		}
		switch request.URL.Path {
		case "/large":
			return response(request, http.StatusOK, "text/plain", strings.Repeat("x", 17)), nil
		case "/slow":
			<-request.Context().Done()
			return nil, request.Context().Err()
		default:
			return response(request, http.StatusOK, "text/plain; charset=utf-8", "safe"), nil
		}
	})
	fetcher, err := New(config(transport))
	if err != nil {
		t.Fatal(err)
	}
	result, err := fetcher.Fetch(context.Background(), "https://files.example.com/safe")
	if err != nil || string(result.Data) != "safe" || result.MediaType != "text/plain" {
		t.Fatal("safe response rejected", err)
	}
	if _, err = fetcher.Fetch(context.Background(), "https://files.example.com/large"); !errors.Is(err, ErrTooLarge) {
		t.Fatal("oversized response accepted", err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	blocking := config(roundTripFunc(func(request *http.Request) (*http.Response, error) {
		close(started)
		select {
		case <-release:
			return response(request, http.StatusOK, "text/plain", "safe"), nil
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
	}))
	blocking.Timeout = time.Second
	fetcher, _ = New(blocking)
	done := make(chan error, 1)
	go func() {
		_, callErr := fetcher.Fetch(context.Background(), "https://files.example.com/safe")
		done <- callErr
	}()
	<-started
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err = fetcher.Fetch(short, "https://files.example.com/safe"); !errors.Is(err, ErrTimeout) {
		t.Fatal("concurrency budget ignored", err)
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	fetcher, _ = New(config(transport))
	if _, err = fetcher.Fetch(context.Background(), "https://files.example.com/slow"); !errors.Is(err, ErrTimeout) {
		t.Fatal("timeout ignored", err)
	}
}

func TestConfigurationFailsClosed(t *testing.T) {
	for _, candidate := range []Config{
		{},
		{AllowedHosts: []string{"127.0.0.1"}, MaxBytes: 1, Timeout: time.Second, Concurrency: 1},
		{AllowedHosts: []string{"*.example.com"}, MaxBytes: 1, Timeout: time.Second, Concurrency: 1},
	} {
		if _, err := New(candidate); !errors.Is(err, ErrPolicy) {
			t.Fatal("unsafe egress configuration accepted", err)
		}
	}
}
