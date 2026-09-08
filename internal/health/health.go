// Package health provides an HTTP health-check handler that runs a set of
// readiness checks (e.g. database connectivity) on each request.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Check reports whether a dependency is healthy.
type Check func(ctx context.Context) error

// Handler returns an http.Handler that runs every check (with a short
// per-request timeout) and responds 200 with {"status":"ok"} if all pass,
// or 503 with per-check error details if any fail.
func Handler(timeout time.Duration, checks map[string]Check) http.Handler {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		failures := map[string]string{}
		for name, check := range checks {
			if err := check(ctx); err != nil {
				failures[name] = err.Error()
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if len(failures) == 0 {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck
			return
		}

		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"status": "unavailable",
			"errors": failures,
		})
	})
}
