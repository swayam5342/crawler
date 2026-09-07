package fetcher

import (
	"context"
	"fmt"
	"net"
	"time"
)

// dialerWithTimeouts returns a DialContext function that resolves DNS with
// its own timeout, distinct from the TCP connect timeout applied to each
// candidate address.
func dialerWithTimeouts(dnsTimeout, connectTimeout time.Duration) func(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("splitting host:port: %w", err)
		}

		dnsCtx, cancel := context.WithTimeout(ctx, dnsTimeout)
		ips, err := net.DefaultResolver.LookupIPAddr(dnsCtx, host)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("dns lookup for %s: %w", host, err)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("dns lookup for %s: no addresses found", host)
		}

		var lastErr error
		for _, ip := range ips {
			connCtx, cancel := context.WithTimeout(ctx, connectTimeout)
			conn, err := dialer.DialContext(connCtx, network, net.JoinHostPort(ip.String(), port))
			cancel()
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, fmt.Errorf("connecting to %s: %w", host, lastErr)
	}
}
