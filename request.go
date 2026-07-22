package httpclient

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"io"
	"math"
	mathrand "math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NewRequest - create a new instance of Request
func NewRequest(address string, timeout time.Duration, tlsConfig *tls.Config, headers Headers) (request *Request) {

	request = new(Request)

	if !strings.HasSuffix(address, "/") {
		address = address + "/" // add an ending '/' if it doesn't exist
	}

	request.Address = address

	request.transport = http.DefaultTransport.(*http.Transport).Clone()
	request.transport.TLSClientConfig = tlsConfig
	request.transport.MaxIdleConns = 100
	request.transport.MaxConnsPerHost = 100
	request.transport.MaxIdleConnsPerHost = 100
	request.transport.ForceAttemptHTTP2 = true // force attempting usage of HTTP/2
	request.suffixEnabled = true

	request.timeout = timeout

	request.conn = &http.Client{Transport: request.transport, Timeout: timeout}

	request.headers = make(http.Header)

	for _, header := range headers {
		request.headers.Set(header.Key, header.Value)
	}

	// Seed a non-global rand source from crypto/rand for use in jitter calculations.
	request.rng = newCryptoSeededRand()

	return
}

// newCryptoSeededRand creates a new rand.Rand seeded from crypto/rand.
// Falls back to a time-based seed only if crypto/rand is unavailable.
func newCryptoSeededRand() *mathrand.Rand {
	var seed int64
	if err := binary.Read(rand.Reader, binary.LittleEndian, &seed); err != nil {
		seed = time.Now().UnixNano()
	}
	return mathrand.New(mathrand.NewSource(seed)) //nolint:gosec
}

// Get - connect to the service with the given data using the GET HTTP request verb
func (request *Request) Get(ctx context.Context, endpoint string) (httpResponse *Response, err error) {
	return request.connect(ctx, "GET", endpoint, nil)
}

// Query - sends an HTTP QUERY request (RFC 10008) through the existing client
// pipeline (see connect). This does not by itself guarantee full RFC 10008
// redirect semantics; net/http's default redirect handling predates the
// QUERY method — see TestQueryRedirectMethodPreservation.
func (request *Request) Query(ctx context.Context, endpoint string, payload io.Reader) (httpResponse *Response, err error) {
	return request.connect(ctx, "QUERY", endpoint, payload)
}

// Post - connect to the service with the given data using the POST HTTP request verb
func (request *Request) Post(ctx context.Context, endpoint string, payload io.Reader) (httpResponse *Response, err error) {
	return request.connect(ctx, "POST", endpoint, payload)
}

// Put - connect to the service with the given data using the PUT HTTP request verb
func (request *Request) Put(ctx context.Context, endpoint string, payload io.Reader) (httpResponse *Response, err error) {
	return request.connect(ctx, "PUT", endpoint, payload)
}

// Patch - connect to the service with the given data using the PATCH HTTP request verb
func (request *Request) Patch(ctx context.Context, endpoint string, payload io.Reader) (httpResponse *Response, err error) {
	return request.connect(ctx, "PATCH", endpoint, payload)
}

// Delete - connect to the service with the given data using the DELETE HTTP request verb
func (request *Request) Delete(ctx context.Context, endpoint string) (httpResponse *Response, err error) {
	return request.connect(ctx, "DELETE", endpoint, nil)
}

// Options - connect to the service with the given data using the OPTIONS HTTP request verb
func (request *Request) Options(ctx context.Context, endpoint string) (httpResponse *Response, err error) {
	return request.connect(ctx, "OPTIONS", endpoint, nil)
}

// Custom - connect to the service with the given data using a custom HTTP request verb
func (request *Request) Custom(ctx context.Context, httpVerb, endpoint string, payload io.Reader) (httpResponse *Response, err error) {
	return request.connect(ctx, httpVerb, endpoint, payload)
}

// SetHeader - sets additional header for the client
func (request *Request) SetHeader(key, value string) {
	request.headers.Set(key, value)
}

// GetHeader - gets a header specified by the key
func (request *Request) GetHeader(key string) (value string) {
	return request.headers.Get(key)
}

// SetUserAgent - sets a custom user agent for the client
func (request *Request) SetUserAgent(useragent string) {
	request.SetHeader("user-agent", useragent)
}

// SetBasicAuth - sets the Authorization header using HTTP Basic authentication.
// Any previously set Authorization header is overwritten.
func (request *Request) SetBasicAuth(username, password string) {
	request.headers.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(username+":"+password)))
}

// SetBearerToken - sets the Authorization header to a Bearer token.
// Any previously set Authorization header is overwritten.
func (request *Request) SetBearerToken(token string) {
	request.headers.Set("Authorization", "Bearer "+token)
}

// SetRateLimit configures a token-bucket rate limiter on the Request.
// limit is the maximum number of requests per second; burst is the maximum burst size.
// Subsequent calls to connect will wait for a token before each HTTP request.
func (request *Request) SetRateLimit(limit Limit, burst int) {
	request.RateLimiter = NewLimiter(limit, burst)
}

// Clone returns a deep copy of the Request. The clone has:
//   - Its own transport and HTTP client (independent TLS and connection pool).
//   - Its own copy of headers (mutating clone headers does not affect the original).
//   - Its own circuit breaker state (starts fresh; failures do not cross-pollinate).
//   - Its own random source seeded from crypto/rand.
//   - A shared RateLimiter pointer (intentional — clone and original share token quota).
//   - Copied scalar fields (Address, Suffix, suffixEnabled, timeout, RetryConfig, CircuitBreaker config).
func (request *Request) Clone() *Request {
	clone := new(Request)

	clone.Address = request.Address
	clone.Suffix = request.Suffix
	clone.suffixEnabled = request.suffixEnabled
	clone.timeout = request.timeout

	// Independent transport and HTTP client.
	clone.transport = request.transport.Clone()
	clone.conn = &http.Client{Transport: clone.transport, Timeout: clone.timeout}

	// Independent header map.
	clone.headers = request.headers.Clone()

	// RateLimiter is shared intentionally — clone and original share the same token quota.
	clone.RateLimiter = request.RateLimiter

	// Copy retry and circuit breaker configuration.
	clone.RetryConfig = request.RetryConfig
	clone.CircuitBreaker = request.CircuitBreaker
	// breaker (circuit state) is not shared — clone starts with a zero-value breaker.

	// Give the clone its own crypto/rand-seeded random source to avoid data races.
	clone.rng = newCryptoSeededRand()

	return clone
}

// SetProxy configures the transport to route requests through the given proxy URL.
// Passing nil reverts to the default behaviour (http.ProxyFromEnvironment).
// The internal HTTP client is rebuilt after the proxy is set so that the change
// takes effect immediately.
func (request *Request) SetProxy(proxyURL *url.URL) {
	if proxyURL == nil {
		request.transport.Proxy = http.ProxyFromEnvironment
	} else {
		request.transport.Proxy = http.ProxyURL(proxyURL)
	}
	request.conn = &http.Client{Transport: request.transport, Timeout: request.timeout}
}

// SetSuffix - sets a base suffix for all endpoint operations
func (request *Request) SetSuffix(suffix string) {

	if !strings.HasSuffix(suffix, "/") {
		suffix = suffix + "/" // add an ending '/' if it doesn't already exist for the suffix
	}

	request.Suffix = strings.TrimPrefix(suffix, "/")
	//request.Suffix = strings.TrimPrefix(suffix, "/") // remove leading '/' if it exists in the suffix
}

// EnableSuffix - temporarily enables or disables base suffix for a call
func (request *Request) EnableSuffix(enabled bool) {
	request.suffixEnabled = enabled
}

// GetTLSConfig - returns currently used `*tls.Config`
func (request *Request) GetTLSConfig() (tlsConfig *tls.Config) {
	return request.transport.TLSClientConfig
}

// SetTLSConfig - overrides existing TLS configuration with a new one; downgrades to HTTP/1.1 because HTTP/2 forbids TLS renegotiation (RFC 7540 §9.2.1)
func (request *Request) SetTLSConfig(tlsConfig *tls.Config) {
	tlsConfig = tlsConfig.Clone() // clone to avoid mutating the caller's config
	tlsConfig.Renegotiation = tls.RenegotiateOnceAsClient

	tr := request.transport

	tr.TLSClientConfig = tlsConfig
	tr.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{} // disable HTTP/2; it forbids renegotiation
	tr.CloseIdleConnections()

	request.conn = &http.Client{Transport: tr, Timeout: request.timeout}
}

// connect - execute the connection
func (request *Request) connect(ctx context.Context, method, endpoint string, payload io.Reader) (response *Response, err error) {

	// Substitute a background context if nil is passed, rather than panicking.
	if ctx == nil {
		ctx = context.Background()
	}

	// Build the full URL.
	var sb strings.Builder
	sb.WriteString(request.Address)
	if request.suffixEnabled && request.Suffix != "" {
		sb.WriteString(request.Suffix)
	}
	// Trim any leading '/' from endpoint.
	sb.WriteString(strings.TrimLeft(endpoint, "/"))
	address := sb.String()

	// --- Circuit breaker pre-check ---
	cb := &request.breaker
	cbCfg := &request.CircuitBreaker
	if cbCfg.Enabled {
		cb.mu.Lock()
		switch cb.state {
		case cbOpen:
			if time.Since(cb.lastFailure) >= cbCfg.ResetTimeout {
				cb.state = cbHalfOpen
			} else {
				cb.mu.Unlock()
				return nil, ErrCircuitOpen
			}
		}
		cb.mu.Unlock()
	}

	// --- Determine retry parameters ---
	retryCfg := &request.RetryConfig
	maxAttempts := max(retryCfg.MaxAttempts, 1)

	retryOn := retryCfg.RetryOn
	if retryOn == nil && maxAttempts > 1 {
		retryOn = defaultRetryOn()
	}

	failOn := cbCfg.FailOn
	if failOn == nil {
		if retryOn != nil {
			failOn = retryOn
		} else {
			failOn = defaultRetryOn()
		}
	}

	baseDelay := retryCfg.BaseDelay
	if baseDelay == 0 && maxAttempts > 1 {
		baseDelay = 500 * time.Millisecond
	}
	maxDelay := retryCfg.MaxDelay
	if maxDelay == 0 && maxAttempts > 1 {
		maxDelay = 30 * time.Second
	}
	multiplier := retryCfg.Multiplier
	if multiplier == 0 && maxAttempts > 1 {
		multiplier = 2.0
	}

	// seekPayload is non-nil when the payload can be rewound between retries.
	var seekPayload io.ReadSeeker
	if payload != nil {
		seekPayload, _ = payload.(io.ReadSeeker)
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Rewind the payload before each attempt after the first.
		if attempt > 1 && payload != nil {
			if seekPayload == nil {
				// Cannot rewind; stop retrying.
				break
			}
			if _, seekErr := seekPayload.Seek(0, io.SeekStart); seekErr != nil {
				break
			}
		}

		// Rate limiter: wait for a token before sending the request.
		if request.RateLimiter != nil {
			if waitErr := request.RateLimiter.Wait(ctx); waitErr != nil {
				err = waitErr
				return
			}
		}

		var httpRequest *http.Request
		httpRequest, err = http.NewRequestWithContext(ctx, method, address, payload)
		if err != nil {
			return
		}
		httpRequest.Header = request.headers.Clone()

		var httpResponse *http.Response
		httpResponse, err = request.conn.Do(httpRequest)

		// --- Record outcome in circuit breaker ---
		if cbCfg.Enabled {
			failed := err != nil || (httpResponse != nil && containsStatus(failOn, httpResponse.StatusCode))
			cb.mu.Lock()
			if failed {
				cb.failures++
				cb.lastFailure = time.Now()
				if cb.state == cbHalfOpen || cb.failures >= cbCfg.MaxFailures {
					cb.state = cbOpen
				}
			} else {
				cb.failures = 0
				cb.state = cbClosed
			}
			cb.mu.Unlock()
		}

		if err != nil {
			// Transport error — do not retry.
			return
		}

		// Check whether we should retry this status code.
		shouldRetry := attempt < maxAttempts && containsStatus(retryOn, httpResponse.StatusCode)

		if !shouldRetry {
			// Success path or final attempt — build and return the response.
			defer httpResponse.Body.Close()

			response = new(Response)
			response.StatusCode = httpResponse.StatusCode

			_, err = io.Copy(&response.Buffer, httpResponse.Body)
			if err != nil {
				return
			}

			response.headers = httpResponse.Header
			response.TLS = httpResponse.TLS
			return
		}

		// Discard the retryable response body before sleeping.
		httpResponse.Body.Close()

		// Compute backoff delay: min(baseDelay * multiplier^(attempt-1), maxDelay).
		delay := time.Duration(float64(baseDelay) * math.Pow(multiplier, float64(attempt-1)))
		if maxDelay > 0 && delay > maxDelay {
			delay = maxDelay
		}
		if retryCfg.Jitter && request.rng != nil {
			// ±50% jitter: delay * (0.5 + random[0,1))
			delay = time.Duration(float64(delay) * (0.5 + request.rng.Float64()))
		}

		// Sleep for the backoff duration, but respect context cancellation.
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		case <-time.After(delay):
		}
	}

	return
}
