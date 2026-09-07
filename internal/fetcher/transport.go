package fetcher

import (
	"crypto/tls"
	"net/http"
	"time"
)

// buildTransports returns two RoundTrippers sharing the same dial/timeout
// configuration: h2 allows HTTP/2 negotiation (Go's default), h1Only
// disables it so a request can be retried over plain HTTP/1.1 if HTTP/2
// fails.
func buildTransports(dnsTimeout, connectTimeout, tlsTimeout time.Duration) (h2, h1Only *http.Transport) {
	dial := dialerWithTimeouts(dnsTimeout, connectTimeout)

	h2 = &http.Transport{
		DialContext:         dial,
		TLSHandshakeTimeout: tlsTimeout,
		ForceAttemptHTTP2:   true,
	}

	h1Only = &http.Transport{
		DialContext:         dial,
		TLSHandshakeTimeout: tlsTimeout,
		ForceAttemptHTTP2:   false,
		// A non-nil, empty TLSNextProto map disables the transport's
		// automatic HTTP/2 upgrade via ALPN.
		TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{},
	}

	return h2, h1Only
}
