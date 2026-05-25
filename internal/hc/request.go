package hc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

type requestOptions struct {
	runID       string
	diagnostics string
}

// WithRunID sets the run ID for the request.
func WithRunID(runID uuid.UUID) RequestOption {
	return func(opts *requestOptions) {
		opts.runID = runID.String()
	}
}

// WithDiagnostics sets the diagnostics string for the request.
func WithDiagnostics(diagnostics string) RequestOption {
	return func(opts *requestOptions) {
		opts.diagnostics = diagnostics
	}
}

// RequestOption is a function that modifies the request behavior of an individual check call.
type RequestOption func(*requestOptions)

func addRunID(urlStr, runID string) (string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("parse URL: %w", err)
	}

	query := parsedURL.Query()
	query.Set("rid", runID)
	parsedURL.RawQuery = query.Encode()

	return parsedURL.String(), nil
}

func simpleHandleURL(ctx context.Context, urlStr string, opts ...RequestOption) error {
	options := new(requestOptions)
	for _, opt := range opts {
		opt(options)
	}

	if options.runID != "" {
		var err error
		urlStr, err = addRunID(urlStr, options.runID)
		if err != nil {
			return BadConfigError{Message: fmt.Sprintf("failed to construct valid HTTP Request URL from %q: %+v", urlStr, err)}
		}
	}
	var body io.Reader = http.NoBody
	if options.diagnostics != "" {
		body = strings.NewReader(options.diagnostics)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, body)
	if err != nil {
		return BadConfigError{Message: fmt.Sprintf("failed to construct valid HTTP Request to %q: %+v", urlStr, err)}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return RequestFailedError{Req: req, Err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return BadStatusError{Req: req, StatusCode: resp.StatusCode}
	}

	return nil
}
