package fetcher

import (
	"errors"
	"fmt"
)

var (
	// ErrBodyTooLarge is returned when a response body exceeds the
	// configured maximum size.
	ErrBodyTooLarge = errors.New("fetcher: response body exceeds max size")
	// ErrTooManyRedirects is returned when a redirect chain exceeds the
	// configured maximum number of hops.
	ErrTooManyRedirects = errors.New("fetcher: too many redirects")
	// ErrRedirectLoop is returned when a redirect chain revisits a URL it
	// has already followed.
	ErrRedirectLoop = errors.New("fetcher: redirect loop detected")
	// ErrCircuitOpen is returned when the circuit breaker for the target
	// domain is open and the request was not attempted.
	ErrCircuitOpen = errors.New("fetcher: circuit breaker open for domain")
	// ErrUnsupportedEncoding is returned when Content-Encoding names a
	// scheme we don't know how to decode.
	ErrUnsupportedEncoding = errors.New("fetcher: unsupported content-encoding")
)

// HTTPStatusError wraps a non-2xx HTTP response.
type HTTPStatusError struct {
	StatusCode int
	Status     string
}

func (e *HTTPStatusError) Error() string {
	return "fetcher: unexpected HTTP status " + e.Status
}

// FetchError reports that Fetch gave up on a URL after exhausting its
// retry budget (or hitting a non-retryable failure), including how many
// attempts were made. Callers can use Attempts when logging to a
// dead-letter queue.
type FetchError struct {
	URL      string
	Attempts int
	Err      error
}

func (e *FetchError) Error() string {
	return fmt.Sprintf("fetcher: giving up on %s after %d attempt(s): %v", e.URL, e.Attempts, e.Err)
}

func (e *FetchError) Unwrap() error {
	return e.Err
}

// AttemptCount reports how many attempts were made before giving up. It
// lets callers (e.g. a dead-letter sink) record attempt counts without
// importing this package, via structural typing.
func (e *FetchError) AttemptCount() int {
	return e.Attempts
}
