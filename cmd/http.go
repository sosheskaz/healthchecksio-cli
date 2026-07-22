package cmd

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/sosheskaz/healthchecksio-cli/internal/hc"
)

const (
	defaultAttempts          = 5
	defaultRetryMaxBackoff   = 30 * time.Second
	defaultConnectionTimeout = 5 * time.Second
	defaultTotalPingTimeout  = 5 * time.Minute
)

var (
	errNegativeAttempts       = errors.New("attempts must not be negative")
	errNonPositiveBackoff     = errors.New("retry maximum backoff must be positive")
	errNonPositiveConnection  = errors.New("connection timeout must be positive")
	errNonPositivePingTimeout = errors.New("total ping timeout must be positive")
)

type pingOptions struct {
	attempts          int
	retryMaxBackoff   time.Duration
	connectionTimeout time.Duration
	totalPingTimeout  time.Duration
}

type pingClientFactory func(hc.RetryConfig) (*http.Client, error)

func defaultPingOptions() *pingOptions {
	return &pingOptions{
		attempts:          defaultAttempts,
		retryMaxBackoff:   defaultRetryMaxBackoff,
		connectionTimeout: defaultConnectionTimeout,
		totalPingTimeout:  defaultTotalPingTimeout,
	}
}

func (o *pingOptions) validate() error {
	if o.attempts < 0 {
		return errNegativeAttempts
	}
	if o.retryMaxBackoff <= 0 {
		return errNonPositiveBackoff
	}
	if o.connectionTimeout <= 0 {
		return errNonPositiveConnection
	}
	if o.totalPingTimeout <= 0 {
		return errNonPositivePingTimeout
	}
	return nil
}

func (o *pingOptions) newCheck(id uuid.UUID, factory pingClientFactory) (*hc.Check, error) {
	client, err := factory(hc.RetryConfig{
		Attempts:          o.attempts,
		MaxBackoff:        o.retryMaxBackoff,
		ConnectionTimeout: o.connectionTimeout,
	})
	if err != nil {
		return nil, err
	}
	return hc.NewUUIDCheck(id, hc.WithHTTPClient(client))
}

func (o *pingOptions) call(ctx context.Context, callback func(context.Context) error) error {
	pingCtx, cancel := context.WithTimeout(ctx, o.totalPingTimeout)
	defer cancel()
	return callback(pingCtx)
}
