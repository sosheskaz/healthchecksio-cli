// Package hc provides health check functionality for Healthchecks.io.
package hc

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/google/uuid"
)

const (
	// DefaultBaseURL is the default base URL for the healthchecks.io API.
	DefaultBaseURL = "https://hc-ping.com"
)

type checkOptions struct {
	baseURL string
}

// CheckOption is a function that modifies the check options.
type CheckOption func(*checkOptions)

// WithBaseURL sets the base URL for the healthchecks.io API.
func WithBaseURL(baseURL string) CheckOption {
	return func(c *checkOptions) {
		c.baseURL = baseURL
	}
}

// Check represents a health check on a service.
type Check struct {
	checkURL string
}

// NewUUIDCheck creates a new health check for the given UUID.
func NewUUIDCheck(id uuid.UUID, opts ...CheckOption) (*Check, error) {
	if id == uuid.Nil {
		return nil, BadConfigError{Message: "id must not be nil"}
	}
	options := &checkOptions{}
	for _, opt := range opts {
		opt(options)
	}

	baseURL := DefaultBaseURL
	if options.baseURL != "" {
		baseURL = options.baseURL
	}

	checkURL, err := url.JoinPath(baseURL, id.String())
	if err != nil {
		return nil, BadConfigError{Message: fmt.Sprintf("failed to construct URL from %q and %q: %+v", baseURL, id.String(), err)}
	}

	return &Check{
		checkURL: checkURL,
	}, nil
}

// NewSlugCheck creates a new health check for the given ping key and slug.
func NewSlugCheck(pingKey, slug string, opts ...CheckOption) (*Check, error) {
	if pingKey == "" {
		return nil, BadConfigError{Message: "pingKey must not be empty"}
	}
	if slug == "" {
		return nil, BadConfigError{Message: "slug must not be empty"}
	}

	options := &checkOptions{}
	for _, opt := range opts {
		opt(options)
	}

	baseURL := DefaultBaseURL
	if options.baseURL != "" {
		baseURL = options.baseURL
	}

	checkURL, err := url.JoinPath(baseURL, pingKey, slug)
	if err != nil {
		return nil, BadConfigError{Message: fmt.Sprintf("failed to construct URL from %q, pingKey (len %d) and %q: %+v", baseURL, len(pingKey), slug, err)}
	}

	return &Check{
		checkURL: checkURL,
	}, nil
}

// Success sends a success ping to the healthchecks.io API.
func (c *Check) Success(ctx context.Context, opts ...RequestOption) error {
	successURL := c.checkURL
	return simpleHandleURL(ctx, successURL, opts...)
}

// Start sends a start ping to the healthchecks.io API.
func (c *Check) Start(ctx context.Context, opts ...RequestOption) error {
	startURL, err := url.JoinPath(c.checkURL, "start")
	if err != nil {
		return BadConfigError{Message: fmt.Sprintf("failed to construct URL from %q and %q: %+v", c.checkURL, "start", err)}
	}
	return simpleHandleURL(ctx, startURL, opts...)
}

// Failure sends a failure ping to the healthchecks.io API.
func (c *Check) Failure(ctx context.Context, opts ...RequestOption) error {
	failureURL, err := url.JoinPath(c.checkURL, "fail")
	if err != nil {
		return BadConfigError{Message: fmt.Sprintf("failed to construct URL from %q and %q: %+v", c.checkURL, "fail", err)}
	}

	return simpleHandleURL(ctx, failureURL, opts...)
}

// Log sends a log message to the healthchecks.io API.
func (c *Check) Log(ctx context.Context, message string, opts ...RequestOption) error {
	logURL, err := url.JoinPath(c.checkURL, "log")
	if err != nil {
		return BadConfigError{Message: fmt.Sprintf("failed to construct URL from %q and %q: %+v", c.checkURL, "log", err)}
	}

	opts = append(opts, RequestOption(func(o *requestOptions) {
		o.diagnostics = message
	}))

	return simpleHandleURL(ctx, logURL, opts...)
}

// CompleteStatus sends a status ping to the healthchecks.io API with the given exit code.
func (c *Check) CompleteStatus(ctx context.Context, status int, opts ...RequestOption) error {
	completeURL, err := url.JoinPath(c.checkURL, strconv.Itoa(status))
	if err != nil {
		return BadConfigError{Message: fmt.Sprintf("failed to construct URL from %q and %q: %+v", c.checkURL, "complete", err)}
	}

	return simpleHandleURL(ctx, completeURL, opts...)
}
