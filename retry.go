package httpclient

import (
	"sync"
	"time"
)

// RetryConfig controls automatic retry with exponential backoff.
// A zero value disables retry entirely — no overhead is added to connect.
type RetryConfig struct {
	// MaxAttempts is the total number of attempts (including the first).
	// 0 or 1 disables retry.
	MaxAttempts int

	// RetryOn is the list of HTTP status codes that trigger a retry.
	// nil defaults to [429, 500, 502, 503, 504] when MaxAttempts > 1.
	RetryOn []int

	// BaseDelay is the initial backoff delay.
	// Defaults to 500ms when zero and retry is active.
	BaseDelay time.Duration

	// MaxDelay caps the backoff delay.
	// Defaults to 30s when zero and retry is active.
	MaxDelay time.Duration

	// Multiplier is the exponential growth factor.
	// Defaults to 2.0 when zero and retry is active.
	Multiplier float64

	// Jitter adds ±50% randomness to the computed delay.
	Jitter bool
}

// CircuitBreakerConfig controls the per-Request circuit breaker.
// A zero value (Enabled: false) disables the circuit breaker entirely.
type CircuitBreakerConfig struct {
	Enabled bool

	// MaxFailures is the number of consecutive failures required to open the circuit.
	MaxFailures int

	// ResetTimeout is how long the circuit stays open before transitioning to half-open.
	ResetTimeout time.Duration

	// FailOn is the list of HTTP status codes counted as failures.
	// nil defaults to the same list as RetryConfig.RetryOn.
	FailOn []int
}

// cbState is the internal circuit breaker state machine.
type cbState uint8

const (
	cbClosed   cbState = iota // requests flow normally
	cbOpen                    // all requests rejected immediately
	cbHalfOpen                // one probe request allowed to test recovery
)

// circuitBreaker holds the mutable circuit breaker state for a single Request.
// Access is serialised by mu.
type circuitBreaker struct {
	mu          sync.Mutex
	state       cbState
	failures    int
	lastFailure time.Time
}

// defaultRetryOn returns the default set of status codes that trigger a retry.
func defaultRetryOn() []int {
	return []int{429, 500, 502, 503, 504}
}

// containsStatus reports whether code appears in codes.
func containsStatus(codes []int, code int) bool {
	for _, c := range codes {
		if c == code {
			return true
		}
	}
	return false
}
