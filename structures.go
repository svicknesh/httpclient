package httpclient

import (
	"bytes"
	"crypto/tls"
	"math/rand"
	"net/http"
	"time"
)

// Request - client connection for  requests
type Request struct {
	Address       string
	Suffix        string // some applications will have a default suffix, this reduces the typing or configuration
	transport     *http.Transport
	timeout       time.Duration
	conn          *http.Client
	headers       http.Header
	suffixEnabled bool

	// RetryConfig controls automatic retry with exponential backoff.
	// A zero value disables retry.
	RetryConfig RetryConfig

	// CircuitBreaker configures the per-Request circuit breaker.
	// A zero value (Enabled: false) disables the circuit breaker.
	CircuitBreaker CircuitBreakerConfig

	// breaker holds the mutable circuit breaker state; not exported.
	breaker circuitBreaker

	// rng is a non-global random source seeded from crypto/rand for jitter.
	// Not safe for concurrent use; connect uses it only under the retry loop
	// which is serial per request.
	rng *rand.Rand

	// RateLimiter, when non-nil, gates each request through a token-bucket limiter.
	// Set with SetRateLimit. nil means no rate limiting.
	RateLimiter *Limiter

	// checkRedirect holds the caller-configured redirect policy, mirrored onto
	// conn.CheckRedirect. Kept separately so SetProxy/SetTLSConfig/Clone can
	// carry it forward when the internal *http.Client is rebuilt.
	// nil means Go's default redirect behaviour (net/http.Client's built-in policy).
	checkRedirect func(req *http.Request, via []*http.Request) error
}

// Header - additional  headers to set
type Header struct {
	Key   string
	Value string
}

type Headers []Header

// Response - client response from  requests
type Response struct {
	StatusCode int
	Buffer     bytes.Buffer
	headers    http.Header
	TLS        *tls.ConnectionState
	//Bytes      []byte
}

// ContentType - response content type header
type ContentType struct {
	Media    string
	Charset  string
	Boundary string
}
