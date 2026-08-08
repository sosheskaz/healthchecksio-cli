package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sosheskaz/healthchecksio-cli/internal/hc"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

var errTestStdin = errors.New("stdin read failure")

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDefaultPingOptions(t *testing.T) {
	t.Parallel()

	opts := defaultPingOptions()
	want := pingOptions{
		attempts:          5,
		retryMaxBackoff:   30 * time.Second,
		connectionTimeout: 5 * time.Second,
		totalPingTimeout:  5 * time.Minute,
	}
	if *opts != want {
		t.Fatalf("default ping options = %+v, want %+v", opts, want)
	}
}

func TestRootCommandPassesCustomRetryConfiguration(t *testing.T) {
	t.Parallel()

	configCh := make(chan hc.RetryConfig, 1)
	deadlineCh := make(chan time.Time, 1)
	factory := func(config hc.RetryConfig) (*http.Client, error) {
		configCh <- config
		return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			deadline, ok := req.Context().Deadline()
			if !ok {
				t.Error("request context has no deadline")
			}
			deadlineCh <- deadline
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Request:    req,
			}, nil
		})}, nil
	}

	started := time.Now()
	cmd := rootCommandWithClientFactory(factory)
	cmd.SetArgs([]string{
		"--attempts", "0",
		"--retry-max-backoff", "12s",
		"--connection-timeout", "3s",
		"--total-ping-timeout", "7s",
		uuid.MustParse("00000000-0000-4000-8000-000000000009").String(),
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	config := <-configCh
	if config.Attempts != 0 || config.MaxBackoff != 12*time.Second || config.ConnectionTimeout != 3*time.Second {
		t.Fatalf("retry config = %+v, want attempts 0, backoff 12s, connection timeout 3s", config)
	}
	remaining := (<-deadlineCh).Sub(started)
	if remaining < 6*time.Second || remaining > 8*time.Second {
		t.Fatalf("request deadline remaining = %s, want approximately 7s", remaining)
	}
}

func TestRootCommandPassesEnvironmentRetryConfiguration(t *testing.T) {
	checkID := uuid.MustParse("00000000-0000-4000-8000-000000000015")
	t.Setenv(envCheckID, checkID.String())
	t.Setenv(envAttempts, "0")
	t.Setenv(envRetryMaxBackoff, "13s")
	t.Setenv(envConnectionTimeout, "4s")
	t.Setenv(envTotalPingTimeout, "8s")

	configCh := make(chan hc.RetryConfig, 1)
	deadlineCh := make(chan time.Time, 1)
	factory := func(config hc.RetryConfig) (*http.Client, error) {
		configCh <- config
		return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			deadline, ok := req.Context().Deadline()
			if !ok {
				t.Error("request context has no deadline")
			}
			deadlineCh <- deadline
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Request:    req,
			}, nil
		})}, nil
	}

	started := time.Now()
	cmd := rootCommandWithClientFactory(factory)
	cmd.SetArgs([]string{})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	config := <-configCh
	if config.Attempts != 0 || config.MaxBackoff != 13*time.Second || config.ConnectionTimeout != 4*time.Second {
		t.Fatalf("retry config = %+v, want attempts 0, backoff 13s, connection timeout 4s", config)
	}
	remaining := (<-deadlineCh).Sub(started)
	if remaining < 7*time.Second || remaining > 9*time.Second {
		t.Fatalf("request deadline remaining = %s, want approximately 8s", remaining)
	}
}

func TestFlagsOverrideEnvironmentRetryConfiguration(t *testing.T) {
	t.Setenv(envAttempts, "not-an-int")
	t.Setenv(envRetryMaxBackoff, "not-a-duration")
	t.Setenv(envConnectionTimeout, "not-a-duration")
	t.Setenv(envTotalPingTimeout, "not-a-duration")

	configCh := make(chan hc.RetryConfig, 1)
	factory := func(config hc.RetryConfig) (*http.Client, error) {
		configCh <- config
		return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Request:    req,
			}, nil
		})}, nil
	}

	cmd := rootCommandWithClientFactory(factory)
	cmd.SetArgs([]string{
		"--attempts", "2",
		"--retry-max-backoff", "14s",
		"--connection-timeout", "6s",
		"--total-ping-timeout", "9s",
		uuid.MustParse("00000000-0000-4000-8000-000000000016").String(),
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	config := <-configCh
	if config.Attempts != 2 || config.MaxBackoff != 14*time.Second || config.ConnectionTimeout != 6*time.Second {
		t.Fatalf("retry config = %+v, want attempts 2, backoff 14s, connection timeout 6s", config)
	}
}

func TestRootCommandRejectsMalformedEnvironmentRetryConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		environment string
	}{
		{name: "attempts", environment: envAttempts},
		{name: "retry maximum backoff", environment: envRetryMaxBackoff},
		{name: "connection timeout", environment: envConnectionTimeout},
		{name: "total ping timeout", environment: envTotalPingTimeout},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const invalidValue = "sensitive-invalid-value"
			for _, binding := range envFlagBindings {
				t.Setenv(binding.environment, "")
			}
			t.Setenv(tc.environment, invalidValue)

			factoryCalled := false
			cmd := rootCommandWithClientFactory(func(hc.RetryConfig) (*http.Client, error) {
				factoryCalled = true
				return http.DefaultClient, nil
			})
			cmd.SetArgs([]string{uuid.MustParse("00000000-0000-4000-8000-000000000017").String()})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			err := cmd.Execute()
			if err == nil {
				t.Fatal("Execute() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.environment) {
				t.Fatalf("Execute() error = %q, want environment name %q", err, tc.environment)
			}
			if strings.Contains(err.Error(), invalidValue) {
				t.Fatalf("Execute() error exposed environment value: %v", err)
			}
			if factoryCalled {
				t.Fatal("client factory called for invalid configuration")
			}
		})
	}
}

func TestRootCommandNamesSemanticallyInvalidEnvironmentRetryConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		value       string
	}{
		{name: "negative attempts", environment: envAttempts, value: "-1"},
		{name: "zero retry maximum backoff", environment: envRetryMaxBackoff, value: "0s"},
		{name: "zero connection timeout", environment: envConnectionTimeout, value: "0s"},
		{name: "zero total ping timeout", environment: envTotalPingTimeout, value: "0s"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.environment, tc.value)

			factoryCalled := false
			cmd := rootCommandWithClientFactory(func(hc.RetryConfig) (*http.Client, error) {
				factoryCalled = true
				return http.DefaultClient, nil
			})
			cmd.SetArgs([]string{uuid.MustParse("00000000-0000-4000-8000-000000000018").String()})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			err := cmd.Execute()
			if err == nil {
				t.Fatal("Execute() error = nil, want error")
			}
			if !errors.Is(err, errInvalidEnvironmentValue) {
				t.Errorf("Execute() error = %v, want invalid environment value", err)
			}
			if !strings.Contains(err.Error(), tc.environment) {
				t.Errorf("Execute() error = %q, want environment name %q", err, tc.environment)
			}
			if strings.Contains(err.Error(), tc.value) {
				t.Errorf("Execute() error exposed environment value: %v", err)
			}
			if factoryCalled {
				t.Fatal("client factory called for invalid configuration")
			}
		})
	}
}

func TestEmptyEnvironmentRetryConfigurationUsesDefaults(t *testing.T) {
	t.Setenv(envAttempts, "")
	t.Setenv(envRetryMaxBackoff, "")
	t.Setenv(envConnectionTimeout, "")
	t.Setenv(envTotalPingTimeout, "")

	configCh := make(chan hc.RetryConfig, 1)
	deadlineCh := make(chan time.Time, 1)
	factory := func(config hc.RetryConfig) (*http.Client, error) {
		configCh <- config
		return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			deadline, ok := req.Context().Deadline()
			if !ok {
				t.Error("request context has no deadline")
			}
			deadlineCh <- deadline
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Request:    req,
			}, nil
		})}, nil
	}

	started := time.Now()
	cmd := rootCommandWithClientFactory(factory)
	cmd.SetArgs([]string{uuid.MustParse("00000000-0000-4000-8000-000000000019").String()})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	config := <-configCh
	if config.Attempts != defaultAttempts || config.MaxBackoff != defaultRetryMaxBackoff || config.ConnectionTimeout != defaultConnectionTimeout {
		t.Fatalf("retry config = %+v, want defaults", config)
	}
	remaining := (<-deadlineCh).Sub(started)
	if remaining < defaultTotalPingTimeout-time.Second || remaining > defaultTotalPingTimeout+time.Second {
		t.Fatalf("request deadline remaining = %s, want approximately %s", remaining, defaultTotalPingTimeout)
	}
}

func TestRootCommandRejectsInvalidPingOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "negative attempts", args: []string{"--attempts", "-1"}},
		{name: "zero backoff", args: []string{"--retry-max-backoff", "0s"}},
		{name: "zero connection timeout", args: []string{"--connection-timeout", "0s"}},
		{name: "zero total ping timeout", args: []string{"--total-ping-timeout", "0s"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			factoryCalled := false
			factory := func(hc.RetryConfig) (*http.Client, error) {
				factoryCalled = true
				return http.DefaultClient, nil
			}
			cmd := rootCommandWithClientFactory(factory)
			cmd.SetArgs(append(tc.args, uuid.MustParse("00000000-0000-4000-8000-000000000010").String()))
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			if err := cmd.Execute(); err == nil {
				t.Fatal("Execute() error = nil, want error")
			}
			if factoryCalled {
				t.Fatal("client factory called for invalid configuration")
			}
		})
	}
}

func TestPingOptionsCallEnforcesTotalTimeout(t *testing.T) {
	t.Parallel()

	opts := defaultPingOptions()
	opts.totalPingTimeout = 10 * time.Millisecond
	err := opts.call(t.Context(), func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("call() error = %v, want context deadline exceeded", err)
	}
}

func TestRootLogSignal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input        func() io.Reader
		name         string
		wantBody     string
		timeout      time.Duration
		wantRequests int32
		wantErr      bool
	}{
		{
			name:         "body",
			input:        func() io.Reader { return strings.NewReader("diagnostics") },
			timeout:      time.Second,
			wantBody:     "diagnostics",
			wantRequests: 1,
		},
		{
			name: "slow stdin outside ping timeout",
			input: func() io.Reader {
				return &delayedReader{reader: strings.NewReader("slow diagnostics"), delay: 50 * time.Millisecond}
			},
			timeout:      20 * time.Millisecond,
			wantBody:     "slow diagnostics",
			wantRequests: 1,
		},
		{
			name:    "read failure",
			input:   func() io.Reader { return failingReader{err: errTestStdin} },
			timeout: time.Second,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var requests atomic.Int32
			bodyCh := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("io.ReadAll() error = %v", err)
				}
				bodyCh <- string(body)
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(server.Close)

			checkID := uuid.MustParse("00000000-0000-4000-8000-000000000031")
			cmd := rootCommandWithClientFactory(routeHealthchecksTo(t, server.URL))
			cmd.SetArgs([]string{"--total-ping-timeout", tc.timeout.String(), checkID.String(), "log"})
			cmd.SetIn(tc.input())
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			err := cmd.Execute()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Execute() error = %v, wantErr %v", err, tc.wantErr)
			}
			if got := requests.Load(); got != tc.wantRequests {
				t.Fatalf("requests = %d, want %d", got, tc.wantRequests)
			}
			if tc.wantRequests > 0 {
				if got := <-bodyCh; got != tc.wantBody {
					t.Fatalf("body = %q, want %q", got, tc.wantBody)
				}
			}
		})
	}
}

type delayedReader struct {
	reader *strings.Reader
	delay  time.Duration
	slept  bool
}

func (r *delayedReader) Read(p []byte) (int, error) {
	if !r.slept {
		time.Sleep(r.delay)
		r.slept = true
	}
	//nolint:wrapcheck // io.Reader must return io.EOF unchanged.
	return r.reader.Read(p)
}

type failingReader struct {
	err error
}

func (r failingReader) Read([]byte) (int, error) {
	return 0, r.err
}
