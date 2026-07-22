package hc

import (
	"context"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
)

func TestNewRetryingHTTPClientConfiguresRetriesAndTimeouts(t *testing.T) {
	t.Parallel()

	client, err := NewRetryingHTTPClient(RetryConfig{
		Attempts:          5,
		MaxBackoff:        30 * time.Second,
		ConnectionTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewRetryingHTTPClient() error = %v", err)
	}

	retryTransport, ok := client.Transport.(*retryablehttp.RoundTripper)
	if !ok {
		t.Fatalf("client.Transport = %T, want *retryablehttp.RoundTripper", client.Transport)
	}
	if got, want := retryTransport.Client.RetryMax, 4; got != want {
		t.Fatalf("RetryMax = %d, want %d", got, want)
	}
	if retryTransport.Client.Logger != nil {
		t.Fatalf("Logger = %T, want nil", retryTransport.Client.Logger)
	}
	transport, ok := retryTransport.Client.HTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("HTTPClient.Transport = %T, want *http.Transport", retryTransport.Client.HTTPClient.Transport)
	}
	if got, want := transport.TLSHandshakeTimeout, 5*time.Second; got != want {
		t.Fatalf("TLSHandshakeTimeout = %s, want %s", got, want)
	}
}

func TestNewRetryingHTTPClientRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config RetryConfig
	}{
		{name: "negative attempts", config: RetryConfig{Attempts: -1, MaxBackoff: time.Second, ConnectionTimeout: time.Second}},
		{name: "zero backoff", config: RetryConfig{Attempts: 1, ConnectionTimeout: time.Second}},
		{name: "zero connection timeout", config: RetryConfig{Attempts: 1, MaxBackoff: time.Second}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewRetryingHTTPClient(tc.config); err == nil {
				t.Fatal("NewRetryingHTTPClient() error = nil, want error")
			}
		})
	}
}

func TestRetryMaxUsesTotalAttemptSemantics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		attempts int
		want     int
	}{
		{attempts: 1, want: 0},
		{attempts: 2, want: 1},
		{attempts: 5, want: 4},
	}
	for _, tc := range tests {
		if got := retryMax(tc.attempts); got != tc.want {
			t.Fatalf("retryMax(%d) = %d, want %d", tc.attempts, got, tc.want)
		}
	}
	if got := retryMax(0); got < 1_000_000 {
		t.Fatalf("retryMax(0) = %d, want effectively unlimited", got)
	}
}

func TestJitteredGrowingBackoffStaysWithinBounds(t *testing.T) {
	t.Parallel()

	for _, capDelay := range []time.Duration{time.Nanosecond, 500 * time.Millisecond, time.Second, 3 * time.Second} {
		for attempt := range 8 {
			upper := min(time.Duration(attempt+1)*time.Second, capDelay)
			lower := upper / 2
			for range 100 {
				got := jitteredGrowingBackoff(time.Second, capDelay, attempt, nil)
				if got < lower || got > upper {
					t.Fatalf("cap %s attempt %d backoff = %s, want between %s and %s", capDelay, attempt, got, lower, upper)
				}
			}
		}
	}
}

func TestRetryAfter(t *testing.T) {
	t.Parallel()

	past := time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)
	tests := []struct {
		name     string
		header   string
		want     time.Duration
		status   int
		response bool
		wantOK   bool
	}{
		{name: "nil response"},
		{name: "missing header", response: true, status: http.StatusServiceUnavailable},
		{name: "negative seconds", response: true, status: http.StatusServiceUnavailable, header: "-5"},
		{name: "zero seconds", response: true, status: http.StatusServiceUnavailable, header: "0", wantOK: true},
		{name: "503 seconds", response: true, status: http.StatusServiceUnavailable, header: "5", want: 5 * time.Second, wantOK: true},
		{name: "429 seconds", response: true, status: http.StatusTooManyRequests, header: "7", want: 7 * time.Second, wantOK: true},
		{name: "duration overflow", response: true, status: http.StatusServiceUnavailable, header: "10000000000", want: time.Duration(math.MaxInt64), wantOK: true},
		{name: "integer overflow", response: true, status: http.StatusServiceUnavailable, header: "99999999999999999999"},
		{name: "malformed", response: true, status: http.StatusServiceUnavailable, header: "not-a-number"},
		{name: "ignored success", response: true, status: http.StatusOK, header: "5"},
		{name: "ignored server error", response: true, status: http.StatusInternalServerError, header: "5"},
		{name: "past date", response: true, status: http.StatusServiceUnavailable, header: past, wantOK: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var resp *http.Response
			if tc.response {
				resp = &http.Response{StatusCode: tc.status, Header: http.Header{}}
				if tc.header != "" {
					resp.Header.Set("Retry-After", tc.header)
				}
			}
			got, ok := retryAfter(resp)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("retryAfter() = (%s, %v), want (%s, %v)", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestJitteredGrowingBackoffCapsRetryAfter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		header     string
		maxBackoff time.Duration
		want       time.Duration
	}{
		{name: "seconds", header: "60", maxBackoff: 30 * time.Second, want: 30 * time.Second},
		{name: "sub-second cap", header: "5", maxBackoff: 500 * time.Millisecond, want: 500 * time.Millisecond},
		{name: "future date", header: time.Now().Add(24 * time.Hour).UTC().Format(http.TimeFormat), maxBackoff: 2 * time.Second, want: 2 * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp := &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Header:     http.Header{"Retry-After": []string{tc.header}},
			}
			if got := jitteredGrowingBackoff(time.Second, tc.maxBackoff, 0, resp); got != tc.want {
				t.Fatalf("backoff = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestRetryingHTTPClientRetriesExpectedStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		status       int
		attempts     int
		wantRequests int32
	}{
		{name: "bad request", status: http.StatusBadRequest, attempts: 3, wantRequests: 1},
		{name: "not implemented", status: http.StatusNotImplemented, attempts: 3, wantRequests: 1},
		{name: "too many requests", status: http.StatusTooManyRequests, attempts: 3, wantRequests: 3},
		{name: "internal server error", status: http.StatusInternalServerError, attempts: 3, wantRequests: 3},
		{name: "single attempt", status: http.StatusServiceUnavailable, attempts: 1, wantRequests: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				if tc.status == http.StatusTooManyRequests {
					w.Header().Set("Retry-After", "0")
				}
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(server.Close)

			check := newRetryTestCheck(t, server.URL, tc.attempts)
			err := check.Success(t.Context())
			var statusErr BadStatusError
			if !errors.As(err, &statusErr) {
				t.Fatalf("Success() error = %T %[1]v, want BadStatusError", err)
			}
			if got := requests.Load(); got != tc.wantRequests {
				t.Fatalf("requests = %d, want %d", got, tc.wantRequests)
			}
		})
	}
}

func TestRetryingHTTPClientReplaysRequestBody(t *testing.T) {
	t.Parallel()

	var (
		mu     sync.Mutex
		bodies []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("io.ReadAll() error = %v", err)
		}
		mu.Lock()
		bodies = append(bodies, string(body))
		attempt := len(bodies)
		mu.Unlock()
		if attempt == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	check := newRetryTestCheck(t, server.URL, 2)
	if err := check.Log(t.Context(), "diagnostics"); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if got, want := len(bodies), 2; got != want {
		t.Fatalf("body count = %d, want %d", got, want)
	}
	for i, body := range bodies {
		if body != "diagnostics" {
			t.Fatalf("body %d = %q, want diagnostics", i, body)
		}
	}
}

func TestRetryingHTTPClientRetriesConnectionFailure(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Error("response writer does not support hijacking")
				return
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Errorf("Hijack() error = %v", err)
				return
			}
			_ = conn.Close()
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	check := newRetryTestCheck(t, server.URL, 2)
	if err := check.Success(t.Context()); err != nil {
		t.Fatalf("Success() error = %v", err)
	}
	if got, want := requests.Load(), int32(2); got != want {
		t.Fatalf("requests = %d, want %d", got, want)
	}
}

func TestRetryingHTTPClientAppliesTLSHandshakeTimeout(t *testing.T) {
	t.Parallel()

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- conn
		}
	}()

	const timeout = 20 * time.Millisecond
	client, err := NewRetryingHTTPClient(RetryConfig{
		Attempts:          1,
		MaxBackoff:        time.Second,
		ConnectionTimeout: timeout,
	})
	if err != nil {
		t.Fatalf("NewRetryingHTTPClient() error = %v", err)
	}
	check, err := NewUUIDCheck(
		uuid.MustParse("00000000-0000-4000-8000-000000000011"),
		WithBaseURL("https://"+listener.Addr().String()),
		WithHTTPClient(client),
	)
	if err != nil {
		t.Fatalf("NewUUIDCheck() error = %v", err)
	}

	started := time.Now()
	err = check.Success(t.Context())
	if err == nil {
		t.Fatal("Success() error = nil, want TLS handshake timeout")
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Success() elapsed = %s, want timeout near %s", elapsed, timeout)
	}
	select {
	case conn := <-accepted:
		_ = conn.Close()
	case <-time.After(time.Second):
		t.Fatal("server did not accept connection")
	}
}

func TestRetryingHTTPClientRetriesUntilContextDeadline(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	check := newRetryTestCheckWithBackoff(t, server.URL, 0, 20*time.Millisecond)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := check.Success(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Success() error = %T %[1]v, want context deadline exceeded", err)
	}
	if got := requests.Load(); got < 2 {
		t.Fatalf("requests = %d, want at least 2", got)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Success() elapsed = %s, want prompt cancellation", elapsed)
	}
}

func newRetryTestCheck(t *testing.T, serverURL string, attempts int) *Check {
	t.Helper()
	return newRetryTestCheckWithBackoff(t, serverURL, attempts, time.Nanosecond)
}

func newRetryTestCheckWithBackoff(t *testing.T, serverURL string, attempts int, maxBackoff time.Duration) *Check {
	t.Helper()

	client, err := NewRetryingHTTPClient(RetryConfig{
		Attempts:          attempts,
		MaxBackoff:        maxBackoff,
		ConnectionTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewRetryingHTTPClient() error = %v", err)
	}
	check, err := NewUUIDCheck(
		uuid.MustParse("00000000-0000-4000-8000-000000000008"),
		WithBaseURL(serverURL),
		WithHTTPClient(client),
	)
	if err != nil {
		t.Fatalf("NewUUIDCheck() error = %v", err)
	}
	return check
}
