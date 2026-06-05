package httpclient

// This file provides a minimal token-bucket rate limiter used by the RateLimiter
// field on Request. It mirrors the golang.org/x/time/rate.Limiter API so that
// the calling code remains compatible if the external package is added later.
//
// NOTE: golang.org/x/time/rate could not be downloaded in the build environment
// (TLS certificate verification failure against proxy.golang.org). A thin
// internal implementation is provided here instead. Replace with the external
// package by:
//   1. Running: go get golang.org/x/time/rate
//   2. Replacing the *Limiter field type with *rate.Limiter
//   3. Deleting this file.

import (
	"context"
	"sync"
	"time"
)

// Limit is the maximum event rate in events per second.
type Limit float64

// Inf is the infinite rate limit — every Wait call succeeds immediately.
const Inf Limit = 1<<63 - 1

// Limiter controls how frequently events are allowed using a token-bucket algorithm.
// A nil Limiter allows all events.
type Limiter struct {
	mu        sync.Mutex
	limit     Limit
	burst     int
	tokens    float64
	lastEvent time.Time
}

// NewLimiter returns a new Limiter that allows up to limit events per second
// with a burst of b.
func NewLimiter(r Limit, b int) *Limiter {
	return &Limiter{
		limit:  r,
		burst:  b,
		tokens: float64(b),
	}
}

// Wait blocks until one event may occur, or the context is done.
func (lim *Limiter) Wait(ctx context.Context) error {
	return lim.WaitN(ctx, 1)
}

// WaitN blocks until n events may occur, or the context is done.
func (lim *Limiter) WaitN(ctx context.Context, n int) error {
	if lim == nil {
		return nil
	}
	lim.mu.Lock()
	wait := lim.reserve(n)
	lim.mu.Unlock()

	if wait <= 0 {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}

// reserve computes the wait duration needed for n tokens and updates state.
// Must be called with lim.mu held.
func (lim *Limiter) reserve(n int) time.Duration {
	if lim.limit == Inf {
		return 0
	}

	now := time.Now()
	if !lim.lastEvent.IsZero() {
		elapsed := now.Sub(lim.lastEvent)
		generated := elapsed.Seconds() * float64(lim.limit)
		lim.tokens += generated
		if lim.tokens > float64(lim.burst) {
			lim.tokens = float64(lim.burst)
		}
	}
	lim.lastEvent = now

	if lim.tokens >= float64(n) {
		lim.tokens -= float64(n)
		return 0
	}

	need := float64(n) - lim.tokens
	lim.tokens = 0
	// wait = need / limit seconds
	return time.Duration(need / float64(lim.limit) * float64(time.Second))
}
