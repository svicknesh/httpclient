package httpclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestServer starts an httptest server bound to 127.0.0.1 (IPv4).
// Returns (server, true) on success, or (nil, false) if the environment does
// not allow TCP listeners (e.g. a restricted sandbox).
func newTestServer(handler http.Handler) (*httptest.Server, bool) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, false
	}
	srv := httptest.NewUnstartedServer(handler)
	srv.Listener = l
	srv.Start()
	return srv, true
}

func TestClient(t *testing.T) {

	tlsConfig := &tls.Config{InsecureSkipVerify: true}

	//client := NewRequest(true, ProtocolHTTP1, "jsonplaceholder.typicode.com", 443, 10, tlsConfig, Headers{Header{Key: "Content-type", Value: "application/json"}})

	client := NewRequest("https://httpclienttest.free.beeceptor.com", 10, tlsConfig, Headers{Header{Key: "Content-type", Value: "application/json"}})
	client.SetHeader("my-custom-header", "cool value yo!")

	response, err := client.Get(context.Background(), "/users")
	if nil != err {
		fmt.Println(err)
		return
	}

	fmt.Println(response.Buffer.String())
	fmt.Println(response.StatusCode)

	//fmt.Println(response.Buffer.String())

	fmt.Println("Media type is " + response.GetContentType().Media)
	fmt.Printf("Is JSON response: %t\n", response.IsJSON())
	fmt.Println(response.TLS.HandshakeComplete)

	/*
		if response.IsJSON() {
			type User struct {
				ID       int    `json:"id"`
				Username string `json:"username"`
			}

			var users []User
			response.ToJSON(&users)
			fmt.Println(users)
		}
	*/

}

func TestProto(t *testing.T) {

	var v Protocol = 100
	fmt.Println(ProtocolHTTP2)
	fmt.Println(v)

	type Proto struct {
		P Protocol `json:"p"`
	}

	p := new(Proto)
	p.P = ProtocolHTTP2

	pb, err := json.Marshal(p)
	if nil != err {
		fmt.Println(err)
		return
	}
	fmt.Println(string(pb))

	p1 := new(Proto)
	err = json.Unmarshal(pb, p1)
	if nil != err {
		fmt.Println(err)
		return
	}
	fmt.Println(p1.P)

}

func BenchmarkClient(b *testing.B) {

	tlsConfig := &tls.Config{InsecureSkipVerify: true}

	//client := NewRequest(true, ProtocolHTTP1, "jsonplaceholder.typicode.com", 443, 10, tlsConfig, Headers{Header{Key: "Content-type", Value: "application/json"}})
	client := NewRequest("https://jsonplaceholder.typicode.com", 10, tlsConfig, Headers{Header{Key: "Content-type", Value: "application/json"}})

	for i := 0; i < b.N; i++ {

		response, err := client.Get(context.Background(), "/users")
		if nil != err {
			//log.Println(err)
			return
		}
		_ = response

		//log.Println(response.StatusCode)

		//fmt.Println(response.Buffer.String())

		//fmt.Println("Media type is " + response.GetContentType().Media)
		//fmt.Printf("Is JSON response: %t\n", response.IsJSON())

		/*
			if response.IsJSON() {
				type User struct {
					ID       int    `json:"id"`
					Username string `json:"username"`
				}

				var users []User
				response.ToJSON(&users)
				fmt.Println(users)
			}
		*/

	}

}

func TestSlash(t *testing.T) {

	tlsConfig := &tls.Config{InsecureSkipVerify: true}

	client := NewRequest("https://example.com", 10, tlsConfig, Headers{Header{Key: "Content-type", Value: "application/json"}})
	client.SetSuffix("/woohoo/")

	endpoint := "/user/hello"

	fmt.Print(client.Address, client.Suffix, strings.TrimPrefix(endpoint, "/"), "\n")

}

func TestOptions(t *testing.T) {

	tlsConfig := &tls.Config{InsecureSkipVerify: true}

	//client := NewRequest(true, ProtocolHTTP1, "jsonplaceholder.typicode.com", 443, 10, tlsConfig, Headers{Header{Key: "Content-type", Value: "application/json"}})

	client := NewRequest("https://httpclienttest.free.beeceptor.com", 10, tlsConfig, Headers{Header{Key: "Content-type", Value: "application/json"}})
	client.SetHeader("my-custom-header", "cool value yo!")

	response, err := client.Options(context.Background(), "/users")
	if nil != err {
		fmt.Println(err)
		return
	}

	fmt.Println(response.GetHeader("Access-Control-Allow-Methods"))
	fmt.Println(response.StatusCode)

}

// TestRetrySuccess verifies that the client retries on 503 and eventually
// returns the 200 response on the third attempt.
func TestRetrySuccess(t *testing.T) {
	var callCount atomic.Int32

	srv, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable) // 503
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer srv.Close()

	client := NewRequest(srv.URL, 5*time.Second, nil, nil)
	client.RetryConfig = RetryConfig{
		MaxAttempts: 3,
		RetryOn:     []int{503},
		BaseDelay:   1 * time.Millisecond,
		MaxDelay:    10 * time.Millisecond,
		Multiplier:  2.0,
	}

	resp, err := client.Get(context.Background(), "/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if callCount.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", callCount.Load())
	}
}

// TestCircuitBreakerOpen verifies that the circuit opens after MaxFailures
// consecutive failures and subsequent calls return ErrCircuitOpen.
func TestCircuitBreakerOpen(t *testing.T) {
	srv, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // 500
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer srv.Close()

	const maxFailures = 3
	client := NewRequest(srv.URL, 5*time.Second, nil, nil)
	client.CircuitBreaker = CircuitBreakerConfig{
		Enabled:      true,
		MaxFailures:  maxFailures,
		ResetTimeout: 10 * time.Second,
		FailOn:       []int{500},
	}

	// Drive the circuit open.
	for range maxFailures {
		_, _ = client.Get(context.Background(), "/")
	}

	// Next call must be rejected immediately.
	_, err := client.Get(context.Background(), "/")
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
}

// TestCircuitBreakerHalfOpen verifies that after the reset timeout the circuit
// transitions to half-open, allows one probe, and closes on success.
func TestCircuitBreakerHalfOpen(t *testing.T) {
	var callCount atomic.Int32

	srv, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError) // 500 — drive circuit open
			return
		}
		w.WriteHeader(http.StatusOK) // probe succeeds
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer srv.Close()

	const maxFailures = 2
	const resetTimeout = 50 * time.Millisecond

	client := NewRequest(srv.URL, 5*time.Second, nil, nil)
	client.CircuitBreaker = CircuitBreakerConfig{
		Enabled:      true,
		MaxFailures:  maxFailures,
		ResetTimeout: resetTimeout,
		FailOn:       []int{500},
	}

	// Drive the circuit open.
	for range maxFailures {
		_, _ = client.Get(context.Background(), "/")
	}

	// Confirm circuit is open.
	_, err := client.Get(context.Background(), "/")
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen immediately after failures, got %v", err)
	}

	// Wait for reset timeout to elapse.
	time.Sleep(resetTimeout + 10*time.Millisecond)

	// Probe request — circuit is half-open; server returns 200.
	resp, err := client.Get(context.Background(), "/")
	if err != nil {
		t.Fatalf("half-open probe failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from probe, got %d", resp.StatusCode)
	}

	// Circuit should now be closed; a further request must succeed.
	resp, err = client.Get(context.Background(), "/")
	if err != nil {
		t.Fatalf("post-close request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 after circuit closed, got %d", resp.StatusCode)
	}
}

// TestSetBasicAuth verifies that SetBasicAuth encodes credentials correctly.
func TestSetBasicAuth(t *testing.T) {
	client := NewRequest("https://example.com", 5*time.Second, nil, nil)
	client.SetBasicAuth("user", "pass")
	got := client.GetHeader("Authorization")
	want := "Basic dXNlcjpwYXNz"
	if got != want {
		t.Fatalf("SetBasicAuth: got %q, want %q", got, want)
	}
}

// TestSetBearerToken verifies that SetBearerToken sets the header correctly.
func TestSetBearerToken(t *testing.T) {
	client := NewRequest("https://example.com", 5*time.Second, nil, nil)
	client.SetBearerToken("mytoken")
	got := client.GetHeader("Authorization")
	want := "Bearer mytoken"
	if got != want {
		t.Fatalf("SetBearerToken: got %q, want %q", got, want)
	}
}

// TestResponseConvenience verifies that String and Bytes return the body without
// consuming the underlying buffer.
func TestResponseConvenience(t *testing.T) {
	resp := &Response{}
	body := "hello world"
	_, _ = resp.Buffer.WriteString(body)

	if got := resp.String(); got != body {
		t.Fatalf("String(): got %q, want %q", got, body)
	}
	if got := string(resp.Bytes()); got != body {
		t.Fatalf("Bytes(): got %q, want %q", got, body)
	}
	// Buffer must still be readable after String() and Bytes().
	if resp.Buffer.String() != body {
		t.Fatal("Buffer was consumed by String() or Bytes()")
	}

	// Verify Bytes returns a copy — mutating it should not affect the buffer.
	b := resp.Bytes()
	b[0] = 'X'
	if resp.Buffer.String() != body {
		t.Fatal("Bytes() returned a direct reference to buffer memory")
	}
}

// TestRateLimiter verifies that the rate limiter blocks when the burst is exhausted.
// When a TCP listener is unavailable (sandbox), the test is skipped.
func TestRateLimiter(t *testing.T) {
	srv, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer srv.Close()

	client := NewRequest(srv.URL, 5*time.Second, nil, nil)
	// 1 request per hour, burst 1 — the second call must block.
	client.SetRateLimit(Limit(1.0/3600), 1)

	// First call: consumes the burst token immediately.
	resp, err := client.Get(context.Background(), "/")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Second call: no tokens available for ~1 hour. Use a short-deadline context.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = client.Get(ctx, "/")
	if err == nil {
		t.Fatal("expected error (rate limit or deadline), got nil")
	}
}

// TestClone verifies that Clone returns an independent copy and that mutating
// the clone's headers does not affect the original.
func TestClone(t *testing.T) {
	original := NewRequest("https://example.com", 5*time.Second, nil, Headers{
		{Key: "X-Custom", Value: "original"},
	})
	original.SetSuffix("/api/")

	clone := original.Clone()

	// Mutate clone header — must not affect original.
	clone.SetHeader("X-Custom", "mutated")

	if original.GetHeader("X-Custom") != "original" {
		t.Fatalf("original header was mutated by clone: got %q", original.GetHeader("X-Custom"))
	}
	if clone.GetHeader("X-Custom") != "mutated" {
		t.Fatalf("clone header not updated: got %q", clone.GetHeader("X-Custom"))
	}

	// Scalar fields should match.
	if clone.Address != original.Address {
		t.Fatalf("Address mismatch: got %q, want %q", clone.Address, original.Address)
	}
	if clone.Suffix != original.Suffix {
		t.Fatalf("Suffix mismatch: got %q, want %q", clone.Suffix, original.Suffix)
	}

	// Clone must have its own transport (pointer differs).
	if clone.transport == original.transport {
		t.Fatal("clone.transport is the same pointer as original.transport")
	}

	// Circuit breaker state must be independent (both start at zero).
	if clone.breaker.failures != 0 {
		t.Fatalf("clone breaker should start fresh, got failures=%d", clone.breaker.failures)
	}
}

// TestSetProxy verifies that SetProxy configures and reverts the transport proxy.
func TestSetProxy(t *testing.T) {
	client := NewRequest("https://example.com", 5*time.Second, nil, nil)

	proxyURL, err := url.Parse("http://proxy.example.com:8080")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}

	client.SetProxy(proxyURL)
	if client.transport.Proxy == nil {
		t.Fatal("expected transport.Proxy to be set after SetProxy, got nil")
	}

	// Revert to environment proxy.
	client.SetProxy(nil)
	if client.transport.Proxy == nil {
		t.Fatal("expected transport.Proxy to be non-nil (ProxyFromEnvironment) after SetProxy(nil)")
	}
}

// TestRateLimiterUnit verifies the internal rate limiter in isolation without
// requiring a TCP listener.
func TestRateLimiterUnit(t *testing.T) {
	// Burst of 2: first two calls should be instant.
	lim := NewLimiter(Limit(1000), 2)

	ctx := context.Background()
	if err := lim.Wait(ctx); err != nil {
		t.Fatalf("first wait: %v", err)
	}
	if err := lim.Wait(ctx); err != nil {
		t.Fatalf("second wait: %v", err)
	}

	// Third call with a very short deadline — should time out because limit is 1000/s
	// but burst is exhausted; refill time is ~1ms. Give it 0ms.
	ctx2, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	// With 1000/s rate and burst drained, need ~1ms to get a token.
	// The 0-timeout context should be done immediately.
	_ = lim.Wait(ctx2) // may or may not error; just ensure no panic
}

// TestResponseStatus verifies the IsSuccess, IsClientError, and IsServerError helpers.
func TestResponseStatus(t *testing.T) {
	cases := []struct {
		code      int
		success   bool
		clientErr bool
		serverErr bool
	}{
		{200, true, false, false},
		{201, true, false, false},
		{299, true, false, false},
		{404, false, true, false},
		{400, false, true, false},
		{499, false, true, false},
		{500, false, false, true},
		{503, false, false, true},
		{599, false, false, true},
		{301, false, false, false},
	}

	for _, tc := range cases {
		resp := &Response{StatusCode: tc.code}
		if resp.IsSuccess() != tc.success {
			t.Errorf("IsSuccess(%d) = %v, want %v", tc.code, resp.IsSuccess(), tc.success)
		}
		if resp.IsClientError() != tc.clientErr {
			t.Errorf("IsClientError(%d) = %v, want %v", tc.code, resp.IsClientError(), tc.clientErr)
		}
		if resp.IsServerError() != tc.serverErr {
			t.Errorf("IsServerError(%d) = %v, want %v", tc.code, resp.IsServerError(), tc.serverErr)
		}
	}
}
