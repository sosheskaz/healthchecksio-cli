package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
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

func routeHealthchecksTo(t *testing.T, targetURL string) {
	t.Helper()

	parsed, err := url.Parse(targetURL)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", targetURL, err)
	}

	previousTransport := http.DefaultTransport
	http.DefaultTransport = rewriteTransport{
		target: parsed,
		base:   previousTransport,
	}
	t.Cleanup(func() {
		http.DefaultTransport = previousTransport
	})
}

//nolint:paralleltest // mutates process-wide http.DefaultTransport for command HTTP routing.
func TestRootCommandAcceptsStartSignal(t *testing.T) {
	checkID := uuid.MustParse("00000000-0000-4000-8000-000000000006")
	requestPath := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath <- r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	routeHealthchecksTo(t, server.URL)

	cmd := rootCommand()
	cmd.SetArgs([]string{checkID.String(), "start"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got, want := <-requestPath, "/"+checkID.String()+"/start"; got != want {
		t.Fatalf("request path = %q, want %q", got, want)
	}
}
