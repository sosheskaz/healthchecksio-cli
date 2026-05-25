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

func simpleHandleURL(ctx context.Context, urlStr string, opts ...RequestOption) error {
	options := new(requestOptions)
	for _, opt := range opts {
		opt(options)
	}

	if options.runID != "" {
		urlStr = urlStr + "?rid=" + url.QueryEscape(options.runID)
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
		return RequestFailedError{Req: req}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return RequestFailedError{Req: req, Err: err}
	}

	return nil
}
