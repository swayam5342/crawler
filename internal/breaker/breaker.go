// Package breaker implements a per-domain circuit breaker: after enough
// consecutive failures against a domain, it stops issuing requests to that
// domain for a cooldown period, then allows a limited number of trial
// requests before fully reopening or re-tripping.
package breaker

import (
	"sync"
	"time"
)

// State is a circuit breaker's state.
type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config controls breaker behavior.
type Config struct {
	// FailureThreshold is the number of consecutive failures (in Closed
	// state) that trips the breaker to Open.
	FailureThreshold int
	// CooldownPeriod is how long the breaker stays Open before allowing a
	// trial request in HalfOpen.
	CooldownPeriod time.Duration
	// HalfOpenMaxCalls is how many consecutive trial successes in HalfOpen
	// are required to fully close the breaker. A single failure in
	// HalfOpen immediately reopens it.
	HalfOpenMaxCalls int
}

// Manager tracks one circuit breaker per domain.
type Manager struct {
	cfg Config

	mu       sync.Mutex
	breakers map[string]*breaker
}

// NewManager creates a Manager. Zero-value fields in cfg are replaced with
// safe minimums (threshold 1, cooldown 1s, half-open calls 1).
func NewManager(cfg Config) *Manager {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 1
	}
	if cfg.CooldownPeriod <= 0 {
		cfg.CooldownPeriod = time.Second
	}
	if cfg.HalfOpenMaxCalls <= 0 {
		cfg.HalfOpenMaxCalls = 1
	}
	return &Manager{cfg: cfg, breakers: make(map[string]*breaker)}
}

// Allow reports whether a request to domain may proceed right now. Callers
// must report the outcome via RecordSuccess or RecordFailure.
func (m *Manager) Allow(domain string) bool {
	return m.breakerFor(domain).allow()
}

// RecordSuccess reports that a request to domain succeeded.
func (m *Manager) RecordSuccess(domain string) {
	m.breakerFor(domain).recordSuccess()
}

// RecordFailure reports that a request to domain failed.
func (m *Manager) RecordFailure(domain string) {
	m.breakerFor(domain).recordFailure()
}

// State returns the current state for domain (Closed if never seen).
func (m *Manager) State(domain string) State {
	return m.breakerFor(domain).currentState()
}

func (m *Manager) breakerFor(domain string) *breaker {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.breakers[domain]
	if !ok {
		b = &breaker{cfg: m.cfg}
		m.breakers[domain] = b
	}
	return b
}

// breaker is a single domain's circuit breaker state machine.
type breaker struct {
	cfg Config

	mu                 sync.Mutex
	state              State
	consecutiveFails   int
	consecutiveSuccess int
	openedAt           time.Time
	halfOpenInFlight   bool
}

func (b *breaker) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case Closed:
		return true
	case Open:
		if time.Since(b.openedAt) < b.cfg.CooldownPeriod {
			return false
		}
		b.state = HalfOpen
		b.consecutiveSuccess = 0
		b.halfOpenInFlight = true
		return true
	case HalfOpen:
		if b.halfOpenInFlight {
			return false // only one trial call in flight at a time
		}
		b.halfOpenInFlight = true
		return true
	default:
		return true
	}
}

func (b *breaker) recordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case Closed:
		b.consecutiveFails = 0
	case HalfOpen:
		b.halfOpenInFlight = false
		b.consecutiveSuccess++
		if b.consecutiveSuccess >= b.cfg.HalfOpenMaxCalls {
			b.state = Closed
			b.consecutiveFails = 0
			b.consecutiveSuccess = 0
		}
	case Open:
		// Shouldn't happen (Allow gates calls), but be defensive.
	}
}

func (b *breaker) recordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case Closed:
		b.consecutiveFails++
		if b.consecutiveFails >= b.cfg.FailureThreshold {
			b.trip()
		}
	case HalfOpen:
		b.halfOpenInFlight = false
		b.trip()
	case Open:
		// Shouldn't happen, but be defensive.
	}
}

// trip must be called with b.mu held.
func (b *breaker) trip() {
	b.state = Open
	b.openedAt = time.Now()
	b.consecutiveFails = 0
	b.consecutiveSuccess = 0
	b.halfOpenInFlight = false
}

func (b *breaker) currentState() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}
