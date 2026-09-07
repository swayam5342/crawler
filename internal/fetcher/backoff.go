package fetcher

import (
	"math/rand"
	"time"
)

// backoffDuration computes an exponential backoff with jitter for the given
// 1-indexed attempt number. jitterFactor is applied as +/- a fraction of the
// computed delay (e.g. 0.3 means +/-30%).
func backoffDuration(attempt int, base, max time.Duration, jitterFactor float64) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := min(attempt-1, 30) // guard against overflow from excessive shifting
	d := base << shift
	if d <= 0 || d > max {
		d = max
	}

	if jitterFactor <= 0 {
		return d
	}
	spread := float64(d) * jitterFactor
	jitter := (rand.Float64()*2 - 1) * spread
	result := float64(d) + jitter
	if result < 0 {
		result = 0
	}
	return time.Duration(result)
}
