package crawler

import (
	"context"
	"sync"
)

// PauseGate lets an operator pause and resume a running Pool. While
// paused, workers stop taking new items from the frontier; requests
// already in flight finish normally. The zero value is not usable; use
// NewPauseGate.
type PauseGate struct {
	mu     sync.Mutex
	paused bool
	resume chan struct{} // closed on Resume; replaced on each Pause
}

// NewPauseGate returns a gate in the given initial state.
func NewPauseGate(paused bool) *PauseGate {
	g := &PauseGate{resume: make(chan struct{})}
	if paused {
		g.paused = true
	} else {
		close(g.resume)
	}
	return g
}

// Pause stops workers from taking new work. Returns false if already paused.
func (g *PauseGate) Pause() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.paused {
		return false
	}
	g.paused = true
	g.resume = make(chan struct{})
	return true
}

// Resume lets workers take new work again. Returns false if not paused.
func (g *PauseGate) Resume() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.paused {
		return false
	}
	g.paused = false
	close(g.resume)
	return true
}

// Paused reports whether the gate is currently paused.
func (g *PauseGate) Paused() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.paused
}

// Wait blocks until the gate is open or ctx is done.
func (g *PauseGate) Wait(ctx context.Context) error {
	g.mu.Lock()
	ch := g.resume
	g.mu.Unlock()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
