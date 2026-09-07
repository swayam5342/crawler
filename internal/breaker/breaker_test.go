package breaker

import (
	"testing"
	"time"
)

func TestClosedAllowsUntilThreshold(t *testing.T) {
	m := NewManager(Config{FailureThreshold: 3, CooldownPeriod: time.Hour, HalfOpenMaxCalls: 1})

	for i := range 2 {
		if !m.Allow("example.com") {
			t.Fatalf("expected Allow() to be true before threshold reached (iter %d)", i)
		}
		m.RecordFailure("example.com")
	}
	if m.State("example.com") != Closed {
		t.Fatalf("state = %v, want Closed before threshold reached", m.State("example.com"))
	}

	if !m.Allow("example.com") {
		t.Fatalf("expected Allow() to be true for the 3rd attempt")
	}
	m.RecordFailure("example.com") // 3rd consecutive failure trips the breaker

	if m.State("example.com") != Open {
		t.Fatalf("state = %v, want Open after threshold reached", m.State("example.com"))
	}
}

func TestSuccessResetsConsecutiveFailureCount(t *testing.T) {
	m := NewManager(Config{FailureThreshold: 2, CooldownPeriod: time.Hour, HalfOpenMaxCalls: 1})

	m.Allow("example.com")
	m.RecordFailure("example.com")
	m.Allow("example.com")
	m.RecordSuccess("example.com") // resets the counter

	m.Allow("example.com")
	m.RecordFailure("example.com")
	if m.State("example.com") != Closed {
		t.Fatalf("state = %v, want Closed (failure count should have reset after success)", m.State("example.com"))
	}
}

func TestOpenBlocksUntilCooldownElapses(t *testing.T) {
	m := NewManager(Config{FailureThreshold: 1, CooldownPeriod: 30 * time.Millisecond, HalfOpenMaxCalls: 1})

	m.Allow("example.com")
	m.RecordFailure("example.com") // trips immediately
	if m.State("example.com") != Open {
		t.Fatalf("expected Open state after single failure with threshold 1")
	}
	if m.Allow("example.com") {
		t.Fatalf("expected Allow() to be false immediately after tripping")
	}

	time.Sleep(40 * time.Millisecond)
	if !m.Allow("example.com") {
		t.Fatalf("expected Allow() to be true after cooldown elapses")
	}
	if m.State("example.com") != HalfOpen {
		t.Fatalf("state = %v, want HalfOpen after cooldown elapses", m.State("example.com"))
	}
}

func TestHalfOpenClosesAfterEnoughSuccesses(t *testing.T) {
	m := NewManager(Config{FailureThreshold: 1, CooldownPeriod: time.Millisecond, HalfOpenMaxCalls: 2})

	m.Allow("example.com")
	m.RecordFailure("example.com") // Open
	time.Sleep(5 * time.Millisecond)

	if !m.Allow("example.com") { // -> HalfOpen, trial 1
		t.Fatalf("expected trial call to be allowed")
	}
	m.RecordSuccess("example.com")
	if m.State("example.com") != HalfOpen {
		t.Fatalf("state = %v, want still HalfOpen after 1 of 2 required successes", m.State("example.com"))
	}

	if !m.Allow("example.com") { // trial 2
		t.Fatalf("expected second trial call to be allowed")
	}
	m.RecordSuccess("example.com")
	if m.State("example.com") != Closed {
		t.Fatalf("state = %v, want Closed after HalfOpenMaxCalls consecutive successes", m.State("example.com"))
	}
}

func TestHalfOpenReopensOnFailure(t *testing.T) {
	m := NewManager(Config{FailureThreshold: 1, CooldownPeriod: time.Millisecond, HalfOpenMaxCalls: 3})

	m.Allow("example.com")
	m.RecordFailure("example.com") // Open
	time.Sleep(5 * time.Millisecond)

	if !m.Allow("example.com") { // -> HalfOpen trial
		t.Fatalf("expected trial call to be allowed")
	}
	m.RecordFailure("example.com") // trial fails: reopen
	if m.State("example.com") != Open {
		t.Fatalf("state = %v, want Open after a failed trial in HalfOpen", m.State("example.com"))
	}
}

func TestHalfOpenOnlyAllowsOneInFlightTrial(t *testing.T) {
	m := NewManager(Config{FailureThreshold: 1, CooldownPeriod: time.Millisecond, HalfOpenMaxCalls: 5})

	m.Allow("example.com")
	m.RecordFailure("example.com") // Open
	time.Sleep(5 * time.Millisecond)

	if !m.Allow("example.com") { // first trial in flight
		t.Fatalf("expected first trial call to be allowed")
	}
	if m.Allow("example.com") { // second concurrent trial should be blocked
		t.Fatalf("expected concurrent second trial call to be blocked while one is in flight")
	}
}

func TestDomainsAreIndependent(t *testing.T) {
	m := NewManager(Config{FailureThreshold: 1, CooldownPeriod: time.Hour, HalfOpenMaxCalls: 1})

	m.Allow("a.com")
	m.RecordFailure("a.com")
	if m.State("a.com") != Open {
		t.Fatalf("expected a.com to be Open")
	}
	if m.State("b.com") != Closed {
		t.Fatalf("expected b.com to remain Closed, unaffected by a.com's failures")
	}
	if !m.Allow("b.com") {
		t.Fatalf("expected b.com to still allow requests")
	}
}
