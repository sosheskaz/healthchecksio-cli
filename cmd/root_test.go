package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"uuid"

	"github.com/sosheskaz/healthchecksio-cli/internal/hc"
)

type rewriteTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rewritten := req.Clone(req.Context())
	rewritten.URL.Scheme = t.target.Scheme
	rewritten.URL.Host = t.target.Host
	rewritten.Host = t.target.Host
	resp, err := t.base.RoundTrip(rewritten)
	if err != nil {
		return nil, fmt.Errorf("round trip rewritten healthchecks request: %w", err)
	}
	return resp, nil
}

func routeHealthchecksTo(t *testing.T, targetURL string, base http.RoundTripper) pingClientFactory {
	t.Helper()

	parsed, err := url.Parse(targetURL)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", targetURL, err)
	}

	return func(hc.RetryConfig) (*http.Client, error) {
		return &http.Client{Transport: rewriteTransport{
			target: parsed,
			base:   base,
		}}, nil
	}
}

func TestRootCommandAcceptsStartSignal(t *testing.T) {
	t.Parallel()

	checkID := uuid.MustParse("00000000-0000-4000-8000-000000000006")
	requestPath := make(chan string, 1)
	server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath <- r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	client := server.Client()

	cmd := rootCommandWithClientFactory(routeHealthchecksTo(t, server.URL, client.Transport))
	cmd.SetArgs([]string{checkID.String(), "start"})
	cmd.SetOut(&bytes.Buffer{})
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got, want := <-requestPath, "/"+checkID.String()+"/start"; got != want {
		t.Fatalf("request path = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "calling check with signal start\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
	if strings.Contains(stderr.String(), checkID.String()) {
		t.Fatalf("stderr exposed check ID: %q", stderr.String())
	}
}

func TestRootCommandAcceptsEnvironmentCheckIDAndSignal(t *testing.T) {
	checkID := uuid.MustParse("00000000-0000-4000-8000-000000000012")
	t.Setenv(envCheckID, checkID.String())

	requestPath := make(chan string, 1)
	server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath <- r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	client := server.Client()

	cmd := rootCommandWithClientFactory(routeHealthchecksTo(t, server.URL, client.Transport))
	cmd.SetArgs([]string{"start"})
	cmd.SetOut(&bytes.Buffer{})
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got, want := <-requestPath, "/"+checkID.String()+"/start"; got != want {
		t.Fatalf("request path = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "calling check with signal start\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
	if strings.Contains(stderr.String(), checkID.String()) {
		t.Fatalf("stderr exposed check ID from environment: %q", stderr.String())
	}
}

func TestRootCommandOmitsCheckIDWithoutSignal(t *testing.T) {
	t.Parallel()

	checkID := uuid.MustParse("00000000-0000-4000-8000-000000000022")
	server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	client := server.Client()

	cmd := rootCommandWithClientFactory(routeHealthchecksTo(t, server.URL, client.Transport))
	cmd.SetArgs([]string{checkID.String()})
	cmd.SetOut(&bytes.Buffer{})
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := stderr.String(), "calling check\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
	if strings.Contains(stderr.String(), checkID.String()) {
		t.Fatalf("stderr exposed check ID: %q", stderr.String())
	}
}

func TestRootCommandAcceptsStandardUUIDForms(t *testing.T) {
	t.Parallel()

	const canonical = "00000000-0000-4000-8000-000000000023"
	tests := []struct {
		name    string
		checkID string
	}{
		{name: "canonical", checkID: canonical},
		{name: "braced", checkID: "{" + canonical + "}"},
		{name: "URN", checkID: "urn:uuid:" + canonical},
		{name: "raw hexadecimal", checkID: "00000000000040008000000000000023"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			requestPath := make(chan string, 1)
			cmd := rootCommandWithClientFactory(func(hc.RetryConfig) (*http.Client, error) {
				return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					requestPath <- req.URL.Path
					return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
				})}, nil
			})
			cmd.SetArgs([]string{tc.checkID})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if got, want := <-requestPath, "/"+canonical; got != want {
				t.Fatalf("request path = %q, want %q", got, want)
			}
		})
	}
}

func TestRootCommandRejectsNonstandardUUIDForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		checkID string
	}{
		{name: "uppercase URN prefix", checkID: "URN:UUID:00000000-0000-4000-8000-000000000023"},
		{name: "wrapped without braces", checkID: "x00000000-0000-4000-8000-000000000023y"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			factoryCalled := false
			cmd := rootCommandWithClientFactory(func(hc.RetryConfig) (*http.Client, error) {
				factoryCalled = true
				return http.DefaultClient, nil
			})
			cmd.SetArgs([]string{tc.checkID})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			if err := cmd.Execute(); err == nil {
				t.Fatal("Execute() error = nil, want invalid check ID error")
			}
			if factoryCalled {
				t.Fatal("client factory called for invalid check ID")
			}
		})
	}
}

func TestRootCommandRejectsEmptyPositionalCheckIDWhenEnvironmentIsSet(t *testing.T) {
	t.Setenv(envCheckID, uuid.MustParse("00000000-0000-4000-8000-000000000021").String())

	factoryCalled := false
	cmd := rootCommandWithClientFactory(func(hc.RetryConfig) (*http.Client, error) {
		factoryCalled = true
		return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       http.NoBody,
				Request:    req,
			}, nil
		})}, nil
	})
	cmd.SetArgs([]string{""})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want empty positional check ID error")
	}
	if factoryCalled {
		t.Fatal("client factory called for empty positional check ID")
	}
}

//nolint:paralleltest // The parent sets process-wide environment for every command case.
func TestUtilityCommandsIgnoreEnvironmentConfiguration(t *testing.T) {
	t.Setenv(envAttempts, "invalid")

	tests := []struct {
		name string
		args []string
	}{
		{name: "help flag", args: []string{"--help"}},
		{name: "help command", args: []string{"help"}},
		{name: "completion command", args: []string{"completion", "bash"}},
		{name: "completion protocol", args: []string{"__complete", ""}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			factoryCalled := false
			cmd := rootCommandWithClientFactory(func(hc.RetryConfig) (*http.Client, error) {
				factoryCalled = true
				return http.DefaultClient, nil
			})
			cmd.SetArgs(tc.args)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if factoryCalled {
				t.Fatal("client factory called for utility command")
			}
		})
	}
}

func TestDirectPingArguments(t *testing.T) {
	environmentID := uuid.MustParse("00000000-0000-4000-8000-000000000013").String()
	positionalID := uuid.MustParse("00000000-0000-4000-8000-000000000014").String()

	tests := []struct {
		name        string
		environment string
		arguments   string
		wantCheckID string
		wantSignal  string
		wantSource  string
	}{
		{name: "environment success", environment: environmentID, wantCheckID: environmentID, wantSource: envCheckID},
		{name: "environment signal", environment: environmentID, arguments: "failure", wantCheckID: environmentID, wantSignal: "failure", wantSource: envCheckID},
		{name: "positional override", environment: environmentID, arguments: positionalID, wantCheckID: positionalID},
		{name: "positional check and signal", environment: environmentID, arguments: positionalID + " start", wantCheckID: positionalID, wantSignal: "start"},
		{name: "missing", wantCheckID: ""},
		{name: "invalid positional without environment", arguments: "not-a-uuid", wantCheckID: "not-a-uuid"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envCheckID, tc.environment)

			args := strings.Fields(tc.arguments)
			checkID, signal, source := directPingArguments(args)
			if checkID != tc.wantCheckID || signal != tc.wantSignal || source != tc.wantSource {
				t.Fatalf(
					"directPingArguments(%v) = (%q, %q, %q), want (%q, %q, %q)",
					args,
					checkID,
					signal,
					source,
					tc.wantCheckID,
					tc.wantSignal,
					tc.wantSource,
				)
			}
		})
	}
}

func TestRootCommandRejectsInvalidEnvironmentCheckIDWithoutExposingIt(t *testing.T) {
	const invalidCheckID = "sensitive-invalid-check-id"
	t.Setenv(envCheckID, invalidCheckID)

	factoryCalled := false
	cmd := rootCommandWithClientFactory(func(hc.RetryConfig) (*http.Client, error) {
		factoryCalled = true
		return http.DefaultClient, nil
	})
	cmd.SetArgs([]string{})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !errors.Is(err, errInvalidEnvironmentValue) {
		t.Errorf("Execute() error = %v, want invalid environment value", err)
	}
	if !strings.Contains(err.Error(), envCheckID) {
		t.Errorf("Execute() error = %q, want environment name %q", err, envCheckID)
	}
	if strings.Contains(err.Error(), invalidCheckID) {
		t.Fatalf("Execute() error exposed environment value: %v", err)
	}
	if factoryCalled {
		t.Fatal("client factory called for invalid check ID")
	}
}

func TestRootCommandTreatsEmptyEnvironmentCheckIDAsUnset(t *testing.T) {
	t.Setenv(envCheckID, "")

	factoryCalled := false
	cmd := rootCommandWithClientFactory(func(hc.RetryConfig) (*http.Client, error) {
		factoryCalled = true
		return http.DefaultClient, nil
	})
	cmd.SetArgs([]string{})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want missing check ID error")
	}
	if factoryCalled {
		t.Fatal("client factory called without a check ID")
	}
}
