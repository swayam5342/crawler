package fetcher

import (
	"context"
	"errors"
	"net"
	"net/http"
)

// isRetryableStatus reports whether an HTTP status code represents a
// transient failure worth retrying.
func isRetryableStatus(status int) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	return status >= 500 && status <= 599
}

// isRetryableError reports whether a transport-level error (no HTTP
// response received) represents a transient failure worth retrying.
// context.Canceled is deliberately excluded: it means the caller gave up,
// not that the request should be retried.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true // timeouts and most net.Error conditions are transient
	}
	return false
}

// isBreakerFailure reports whether an outcome (status, err) should count
// against a domain's circuit breaker. Client errors (4xx other than 429)
// indicate a reachable server responding normally, so they don't count as
// breaker failures.
func isBreakerFailure(status int, err error) bool {
	if err != nil {
		return true
	}
	return isRetryableStatus(status)
}
