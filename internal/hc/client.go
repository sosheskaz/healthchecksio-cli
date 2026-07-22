package hc

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

const retryBackoffStep = time.Second

var (
	errNegativeAttempts      = errors.New("attempts must not be negative")
	errNonPositiveBackoff    = errors.New("maximum backoff must be positive")
	errNonPositiveConnection = errors.New("connection timeout must be positive")
	errUnexpectedTransport   = errors.New("unexpected default HTTP transport type")
)

// RetryConfig configures an HTTP client for Healthchecks.io pings.
type RetryConfig struct {
	Attempts          int
	MaxBackoff        time.Duration
	ConnectionTimeout time.Duration
}

// NewRetryingHTTPClient creates an HTTP client configured for retrying pings.
func NewRetryingHTTPClient(config RetryConfig) (*http.Client, error) {
	if config.Attempts < 0 {
		return nil, errNegativeAttempts
	}
	if config.MaxBackoff <= 0 {
		return nil, errNonPositiveBackoff
	}
	if config.ConnectionTimeout <= 0 {
		return nil, errNonPositiveConnection
	}

	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("%w: got %T, want *http.Transport", errUnexpectedTransport, http.DefaultTransport)
	}
	transport := defaultTransport.Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   config.ConnectionTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	transport.TLSHandshakeTimeout = config.ConnectionTimeout

	retryClient := retryablehttp.NewClient()
	retryClient.HTTPClient.Transport = transport
	retryClient.Logger = nil
	retryClient.RetryWaitMin = retryBackoffStep
	retryClient.RetryWaitMax = config.MaxBackoff
	retryClient.RetryMax = retryMax(config.Attempts)
	retryClient.Backoff = jitteredGrowingBackoff
	retryClient.ErrorHandler = retryablehttp.PassthroughErrorHandler

	return retryClient.StandardClient(), nil
}

func retryMax(attempts int) int {
	if attempts == 0 {
		return int(^uint(0) >> 1)
	}
	return attempts - 1
}

func jitteredGrowingBackoff(_, maxBackoff time.Duration, attempt int, resp *http.Response) time.Duration {
	if wait, ok := retryAfter(resp); ok {
		return min(wait, maxBackoff)
	}

	upper := maxBackoff
	if maxSteps := int64(maxBackoff / retryBackoffStep); maxSteps > int64(attempt) {
		upper = time.Duration(attempt+1) * retryBackoffStep
	}
	lower := upper / 2
	//nolint:gosec // Backoff jitter does not require cryptographic randomness.
	return lower + time.Duration(rand.Int64N(int64(upper-lower)+1))
}

func retryAfter(resp *http.Response) (time.Duration, bool) {
	if resp == nil || (resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusServiceUnavailable) {
		return 0, false
	}
	header := resp.Header.Get("Retry-After")
	if header == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(header, 10, 64); err == nil {
		if seconds < 0 {
			return 0, false
		}
		const maxDurationSeconds = int64((1<<63 - 1) / time.Second)
		if seconds > maxDurationSeconds {
			return time.Duration(1<<63 - 1), true
		}
		return time.Duration(seconds) * time.Second, true
	}
	retryAt, err := http.ParseTime(header)
	if err != nil {
		return 0, false
	}
	return max(time.Until(retryAt), 0), true
}
