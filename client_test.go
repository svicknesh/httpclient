package httpclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
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

// TestClient verifies a GET request end-to-end against a local httptest
// server: the outgoing method, path, and headers are exactly what was
// configured, and the response status, body, and content type are read back
// correctly through the normal httpclient response path. A request error
// fails the test rather than being logged and ignored.
func TestClient(t *testing.T) {
	const responseBody = `[{"id":1,"username":"johndoe"}]`

	var mu sync.Mutex
	var gotMethod, gotPath, gotContentType, gotCustomHeader string

	srv, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-type")
		gotCustomHeader = r.Header.Get("my-custom-header")
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(responseBody))
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer srv.Close()

	client := NewRequest(srv.URL, 10*time.Second, nil, Headers{Header{Key: "Content-type", Value: "application/json"}})
	client.SetHeader("my-custom-header", "cool value yo!")

	response, err := client.Get(context.Background(), "/users")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	method, path, contentType, customHeader := gotMethod, gotPath, gotContentType, gotCustomHeader
	mu.Unlock()

	if method != http.MethodGet {
		t.Fatalf("expected method GET, got %q", method)
	}
	if path != "/users" {
		t.Fatalf("expected path /users, got %q", path)
	}
	if contentType != "application/json" {
		t.Fatalf("expected Content-type application/json, got %q", contentType)
	}
	if customHeader != "cool value yo!" {
		t.Fatalf("expected my-custom-header %q, got %q", "cool value yo!", customHeader)
	}

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.StatusCode)
	}
	if response.String() != responseBody {
		t.Fatalf("unexpected body: got %q, want %q", response.String(), responseBody)
	}
	if media := response.GetContentType().Media; media != "application/json" {
		t.Fatalf("expected media type application/json, got %q", media)
	}
	if !response.IsJSON() {
		t.Fatal("expected IsJSON() to be true")
	}
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

// BenchmarkClient measures the cost of a GET request through the normal
// httpclient path against a local httptest server. Setup (server and client
// construction) happens before b.ResetTimer so only the request/response
// cost is measured. A setup or request error fails the benchmark rather than
// returning silently.
func BenchmarkClient(b *testing.B) {
	srv, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	if !ok {
		b.Skip("TCP listener not available in this environment")
	}
	defer srv.Close()

	client := NewRequest(srv.URL, 10*time.Second, nil, Headers{Header{Key: "Content-type", Value: "application/json"}})

	b.ResetTimer()
	for range b.N {
		response, err := client.Get(context.Background(), "/users")
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		if response.StatusCode != http.StatusOK {
			b.Fatalf("unexpected status: %d", response.StatusCode)
		}
	}
}

func TestSlash(t *testing.T) {

	tlsConfig := &tls.Config{InsecureSkipVerify: true}

	client := NewRequest("https://example.com", 10, tlsConfig, Headers{Header{Key: "Content-type", Value: "application/json"}})
	client.SetSuffix("/woohoo/")

	endpoint := "/user/hello"

	fmt.Print(client.Address, client.Suffix, strings.TrimPrefix(endpoint, "/"), "\n")

}

// TestOptions verifies an OPTIONS request end-to-end against a local
// httptest server: the outgoing method, path, and headers are exactly what
// was configured, and the response status and headers are read back
// correctly. A request error fails the test rather than being logged and
// ignored.
func TestOptions(t *testing.T) {
	var mu sync.Mutex
	var gotMethod, gotPath, gotCustomHeader string

	srv, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCustomHeader = r.Header.Get("my-custom-header")
		mu.Unlock()

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer srv.Close()

	client := NewRequest(srv.URL, 10*time.Second, nil, Headers{Header{Key: "Content-type", Value: "application/json"}})
	client.SetHeader("my-custom-header", "cool value yo!")

	response, err := client.Options(context.Background(), "/users")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	method, path, customHeader := gotMethod, gotPath, gotCustomHeader
	mu.Unlock()

	if method != http.MethodOptions {
		t.Fatalf("expected method OPTIONS, got %q", method)
	}
	if path != "/users" {
		t.Fatalf("expected path /users, got %q", path)
	}
	if customHeader != "cool value yo!" {
		t.Fatalf("expected my-custom-header %q, got %q", "cool value yo!", customHeader)
	}

	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", response.StatusCode)
	}
	if got := response.GetHeader("Access-Control-Allow-Methods"); len(got) == 0 || got[0] != "GET, POST, OPTIONS" {
		t.Fatalf("expected Access-Control-Allow-Methods header, got %v", got)
	}
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

// TestMethodVerbs is a table-driven regression test covering every supported
// HTTP verb method (including QUERY) end-to-end against a local test server.
// It fails if a future change maps QUERY (or any other verb) onto a different
// method, drops its body, loses its query string, or bypasses the normal
// header/response handling pipeline shared by all verb methods.
func TestMethodVerbs(t *testing.T) {
	type captured struct {
		method   string
		path     string
		rawQuery string
		body     string
		headers  http.Header
	}

	var mu sync.Mutex
	var last captured

	srv, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)

		mu.Lock()
		last = captured{
			method:   r.Method,
			path:     r.URL.Path,
			rawQuery: r.URL.RawQuery,
			body:     string(bodyBytes),
			headers:  r.Header.Clone(),
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer srv.Close()

	client := NewRequest(srv.URL, 5*time.Second, nil, Headers{
		{Key: "Content-Type", Value: "application/json"},
	})
	client.SetHeader("X-Custom-Test", "custom-value")
	client.SetBearerToken("test-token")

	cases := []struct {
		name       string
		wantMethod string
		wantBody   string
		call       func(ctx context.Context) (*Response, error)
	}{
		{"GET", "GET", "", func(ctx context.Context) (*Response, error) {
			return client.Get(ctx, "/items?foo=bar")
		}},
		{"POST", "POST", "post-body", func(ctx context.Context) (*Response, error) {
			return client.Post(ctx, "/items?foo=bar", strings.NewReader("post-body"))
		}},
		{"PUT", "PUT", "put-body", func(ctx context.Context) (*Response, error) {
			return client.Put(ctx, "/items?foo=bar", strings.NewReader("put-body"))
		}},
		{"PATCH", "PATCH", "patch-body", func(ctx context.Context) (*Response, error) {
			return client.Patch(ctx, "/items?foo=bar", strings.NewReader("patch-body"))
		}},
		{"DELETE", "DELETE", "", func(ctx context.Context) (*Response, error) {
			return client.Delete(ctx, "/items?foo=bar")
		}},
		{"OPTIONS", "OPTIONS", "", func(ctx context.Context) (*Response, error) {
			return client.Options(ctx, "/items?foo=bar")
		}},
		{"QUERY", "QUERY", "query-body", func(ctx context.Context) (*Response, error) {
			return client.Query(ctx, "/items?foo=bar", strings.NewReader("query-body"))
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := tc.call(context.Background())
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.name, err)
			}

			mu.Lock()
			got := last
			mu.Unlock()

			// The outgoing method must be exactly what was requested — not
			// silently converted to GET, POST, or any other verb.
			if got.method != tc.wantMethod {
				t.Fatalf("%s: server observed method %q, want %q", tc.name, got.method, tc.wantMethod)
			}
			if got.path != "/items" {
				t.Fatalf("%s: server observed path %q, want %q", tc.name, got.path, "/items")
			}
			if got.rawQuery != "foo=bar" {
				t.Fatalf("%s: server observed query %q, want %q", tc.name, got.rawQuery, "foo=bar")
			}
			if got.body != tc.wantBody {
				t.Fatalf("%s: server observed body %q, want %q", tc.name, got.body, tc.wantBody)
			}
			if ct := got.headers.Get("Content-Type"); ct != "application/json" {
				t.Fatalf("%s: server observed Content-Type %q, want %q", tc.name, ct, "application/json")
			}
			if auth := got.headers.Get("Authorization"); auth != "Bearer test-token" {
				t.Fatalf("%s: server observed Authorization %q, want %q", tc.name, auth, "Bearer test-token")
			}
			if custom := got.headers.Get("X-Custom-Test"); custom != "custom-value" {
				t.Fatalf("%s: server observed X-Custom-Test %q, want %q", tc.name, custom, "custom-value")
			}

			// Successful responses must go through the existing response-handling path.
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("%s: expected 200, got %d", tc.name, resp.StatusCode)
			}
			if !resp.IsJSON() {
				t.Fatalf("%s: expected JSON content type", tc.name)
			}
			if resp.String() != `{"status":"ok"}` {
				t.Fatalf("%s: unexpected body %q", tc.name, resp.String())
			}
		})
	}
}

// TestQueryNonSuccessResponse verifies that a non-success response to a QUERY
// request uses the same error-handling behaviour as every other verb: the
// response and its status code are returned normally, with no error, and the
// body remains readable.
func TestQueryNonSuccessResponse(t *testing.T) {
	srv, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer srv.Close()

	client := NewRequest(srv.URL, 5*time.Second, nil, nil)

	resp, err := client.Query(context.Background(), "/missing", strings.NewReader("payload"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	if resp.String() != "not found" {
		t.Fatalf("unexpected body: %q", resp.String())
	}
}

// TestQueryCurrentRedirectBehavior records Go's actual net/http redirect
// behaviour for QUERY across 301, 302, 303, 307, and 308. This client does not
// override http.Client.CheckRedirect, so redirects are handled entirely by
// the Go standard library before connect() ever sees the final response.
//
// RFC 10008 §2.5 ("The HTTP QUERY Method") requires 301, 302, 307, and 308
// responses to a QUERY request to preserve both the QUERY method and the
// enclosed content when the user agent retries against the new target URI;
// only a 303 response should be followed as a GET. Go's net/http predates
// RFC 10008 and applies its pre-existing rule instead (see
// net/http.redirectBehavior, referencing Issue 18570): only GET and HEAD
// survive a 301/302/303 redirect unmodified. Every other method — including
// QUERY — is downgraded to GET with the body dropped. This test proves that
// downgrade happens for 301/302 (a genuine RFC 10008 non-compliance) and,
// coincidentally correctly per the RFC, for 303 too. 307 and 308 do preserve
// the QUERY method and body, but only because this test's payload is a
// *strings.Reader — see TestQueryRedirectBodyReplayRequiresGetBody for the
// case where that does not hold.
func TestQueryCurrentRedirectBehavior(t *testing.T) {
	type captured struct {
		method string
		path   string
		body   string
		auth   string
	}

	cases := []struct {
		name            string
		status          int
		wantFinalMethod string
		wantFinalBody   string
	}{
		{"301_MovedPermanently", http.StatusMovedPermanently, "GET", ""},
		{"302_Found", http.StatusFound, "GET", ""},
		{"303_SeeOther", http.StatusSeeOther, "GET", ""},
		{"307_TemporaryRedirect", http.StatusTemporaryRedirect, "QUERY", "query-payload"},
		{"308_PermanentRedirect", http.StatusPermanentRedirect, "QUERY", "query-payload"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var hops []captured

			srv, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				bodyBytes, _ := io.ReadAll(r.Body)

				mu.Lock()
				hops = append(hops, captured{
					method: r.Method,
					path:   r.URL.Path,
					body:   string(bodyBytes),
					auth:   r.Header.Get("Authorization"),
				})
				mu.Unlock()

				if r.URL.Path == "/redirect" {
					w.Header().Set("Location", "/target")
					w.WriteHeader(tc.status)
					return
				}

				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("final-response"))
			}))
			if !ok {
				t.Skip("TCP listener not available in this environment")
			}
			defer srv.Close()

			client := NewRequest(srv.URL, 5*time.Second, nil, nil)
			client.SetBearerToken("redirect-token")

			resp, err := client.Query(context.Background(), "/redirect", strings.NewReader("query-payload"))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected the redirect to be followed to a final 200, got %d", resp.StatusCode)
			}
			if resp.String() != "final-response" {
				t.Fatalf("expected redirect to be followed to /target, got body %q", resp.String())
			}

			mu.Lock()
			defer mu.Unlock()
			if len(hops) != 2 {
				t.Fatalf("expected 2 request hops (redirect + target), got %d: %+v", len(hops), hops)
			}
			first, final := hops[0], hops[1]

			if first.method != "QUERY" {
				t.Fatalf("first hop: expected method QUERY, got %q", first.method)
			}
			if final.path != "/target" {
				t.Fatalf("final hop: expected path /target, got %q", final.path)
			}
			if final.method != tc.wantFinalMethod {
				t.Fatalf("final hop: status %d — expected method %q, got %q", tc.status, tc.wantFinalMethod, final.method)
			}
			if final.body != tc.wantFinalBody {
				t.Fatalf("final hop: status %d — expected body %q, got %q", tc.status, tc.wantFinalBody, final.body)
			}
			if final.auth != "Bearer redirect-token" {
				t.Fatalf("final hop: Authorization header not preserved across same-host redirect, got %q", final.auth)
			}
		})
	}
}

// TestQueryRedirectBodyReplayRequiresGetBody proves that request-body replay
// on a 307/308 redirect is NOT reliable for every io.Reader accepted by
// Query — only for the concrete types net/http.NewRequestWithContext
// special-cases (*bytes.Buffer, *bytes.Reader, *strings.Reader), which is
// where it populates Request.GetBody. For any other io.Reader — here,
// io.NopCloser wrapping a *strings.Reader, which hides the concrete type the
// switch in NewRequestWithContext matches on — GetBody stays nil. Per
// net/http.redirectBehavior, when GetBody is nil and the request had a body,
// the client deliberately does NOT follow the 307/308 redirect: it returns
// the redirect response to the caller unmodified rather than risk sending a
// bodyless or already-drained request. This is safe (no data corruption) but
// means the caller must be prepared to handle a raw 307/308 Response.
func TestQueryRedirectBodyReplayRequiresGetBody(t *testing.T) {
	srv, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			w.Header().Set("Location", "/target")
			w.WriteHeader(http.StatusTemporaryRedirect)
			_, _ = w.Write([]byte("redirect-body"))
			return
		}
		t.Errorf("redirect target should not be reached when GetBody is unavailable, but got %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer srv.Close()

	client := NewRequest(srv.URL, 5*time.Second, nil, nil)

	// io.NopCloser hides the underlying *strings.Reader behind a type
	// NewRequestWithContext does not recognize, so GetBody is left nil.
	payload := io.NopCloser(strings.NewReader("query-payload"))

	resp, err := client.Query(context.Background(), "/redirect", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The redirect is NOT followed — the 307 response is returned as-is.
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("expected the unfollowed 307 response to be returned, got status %d", resp.StatusCode)
	}
	if resp.String() != "redirect-body" {
		t.Fatalf("unexpected body %q", resp.String())
	}
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

// --- Redirect control tests ---

// TestRedirectDefaultBehaviorUnchanged verifies that a Request created via
// NewRequest, without any redirect-policy call, still follows redirects
// using Go's standard net/http default behaviour.
func TestRedirectDefaultBehaviorUnchanged(t *testing.T) {
	var targetHits atomic.Int32

	target, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("landed"))
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer target.Close()

	origin, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL+"/landing")
		w.WriteHeader(http.StatusFound)
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer origin.Close()

	client := NewRequest(origin.URL, 5*time.Second, nil, nil)

	resp, err := client.Get(context.Background(), "/start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected the default client to follow the redirect to 200, got %d", resp.StatusCode)
	}
	if resp.String() != "landed" {
		t.Fatalf("unexpected body: %q", resp.String())
	}
	if targetHits.Load() != 1 {
		t.Fatalf("expected redirect target to be hit exactly once, got %d", targetHits.Load())
	}
}

// TestDisableRedirects verifies that DisableRedirects prevents the redirect
// destination from ever being contacted and returns the redirect response
// itself as an ordinary, usable Response.
func TestDisableRedirects(t *testing.T) {
	var targetHits atomic.Int32

	target, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer target.Close()

	origin, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL+"/landing")
		w.WriteHeader(http.StatusFound) // 302
		_, _ = w.Write([]byte("redirecting"))
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer origin.Close()

	client := NewRequest(origin.URL, 5*time.Second, nil, nil)
	client.DisableRedirects()

	resp, err := client.Get(context.Background(), "/start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected the unfollowed 302 to be returned, got %d", resp.StatusCode)
	}
	if resp.String() != "redirecting" {
		t.Fatalf("unexpected body: %q", resp.String())
	}
	if got := resp.GetHeader("Location"); len(got) == 0 || got[0] != target.URL+"/landing" {
		t.Fatalf("expected Location header to be preserved, got %v", got)
	}
	if targetHits.Load() != 0 {
		t.Fatalf("redirect destination must never be contacted, got %d hits", targetHits.Load())
	}
}

// TestDisableRedirectsAllStatuses verifies DisableRedirects across every
// redirect status code called out in the requirements: 301, 302, 303, 307, 308.
func TestDisableRedirectsAllStatuses(t *testing.T) {
	statuses := []int{
		http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect,
	}

	for _, status := range statuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var targetHits atomic.Int32

			target, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				targetHits.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			if !ok {
				t.Skip("TCP listener not available in this environment")
			}
			defer target.Close()

			origin, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", target.URL+"/landing")
				w.WriteHeader(status)
			}))
			if !ok {
				t.Skip("TCP listener not available in this environment")
			}
			defer origin.Close()

			client := NewRequest(origin.URL, 5*time.Second, nil, nil)
			client.DisableRedirects()

			resp, err := client.Get(context.Background(), "/start")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.StatusCode != status {
				t.Fatalf("expected status %d, got %d", status, resp.StatusCode)
			}
			if targetHits.Load() != 0 {
				t.Fatalf("redirect destination must never be contacted for status %d, got %d hits", status, targetHits.Load())
			}
		})
	}
}

// TestDisableRedirectsCredentialSafety proves, structurally, that no
// credential — Basic/Bearer Authorization or an arbitrary custom header such
// as X-Api-Key — ever reaches a redirect destination once DisableRedirects
// is set. The destination handler receives zero requests, so it cannot see
// the credential header even if it wanted to.
func TestDisableRedirectsCredentialSafety(t *testing.T) {
	cases := []struct {
		name      string
		setup     func(c *Request)
		headerKey string
	}{
		{"BasicAuth", func(c *Request) { c.SetBasicAuth("user", "pass") }, "Authorization"},
		{"BearerToken", func(c *Request) { c.SetBearerToken("secret-token") }, "Authorization"},
		{"CustomAPIKey", func(c *Request) { c.SetHeader("X-Api-Key", "super-secret") }, "X-Api-Key"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var targetHits atomic.Int32
			var targetSawHeader atomic.Bool

			target, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				targetHits.Add(1)
				if r.Header.Get(tc.headerKey) != "" {
					targetSawHeader.Store(true)
				}
				w.WriteHeader(http.StatusOK)
			}))
			if !ok {
				t.Skip("TCP listener not available in this environment")
			}
			defer target.Close()

			origin, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", target.URL+"/landing")
				w.WriteHeader(http.StatusFound)
			}))
			if !ok {
				t.Skip("TCP listener not available in this environment")
			}
			defer origin.Close()

			client := NewRequest(origin.URL, 5*time.Second, nil, nil)
			tc.setup(client)
			client.DisableRedirects()

			_, err := client.Get(context.Background(), "/start")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if targetHits.Load() != 0 {
				t.Fatalf("redirect destination must receive zero requests, got %d", targetHits.Load())
			}
			if targetSawHeader.Load() {
				t.Fatal("redirect destination must never see the credential header")
			}
		})
	}
}

// TestSetCheckRedirectCustomPolicy verifies that a caller-installed policy is
// actually invoked, can allow one redirect and reject the next, and that the
// destinations it was invoked for are exactly what's expected.
func TestSetCheckRedirectCustomPolicy(t *testing.T) {
	var mu sync.Mutex
	var seenVia []string

	final, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("final hop must never be reached: custom policy should reject the second redirect")
		w.WriteHeader(http.StatusOK)
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer final.Close()

	hop2, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", final.URL+"/final")
		w.WriteHeader(http.StatusFound)
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer hop2.Close()

	origin, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", hop2.URL+"/hop2")
		w.WriteHeader(http.StatusFound)
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer origin.Close()

	errTooManyRedirects := errors.New("custom policy: only one redirect allowed")

	client := NewRequest(origin.URL, 5*time.Second, nil, nil)
	client.SetCheckRedirect(func(req *http.Request, via []*http.Request) error {
		mu.Lock()
		seenVia = append(seenVia, req.URL.String())
		mu.Unlock()
		// via holds every request already made, oldest first. On the first
		// invocation (allowing the origin -> hop2 redirect) via has length 1
		// (just the original request). Reject once a second redirect is
		// attempted (hop2 -> final).
		if len(via) >= 2 {
			return errTooManyRedirects
		}
		return nil
	})

	_, err := client.Get(context.Background(), "/start")
	if err == nil {
		t.Fatal("expected an error from the second redirect being rejected, got nil")
	}
	if !errors.Is(err, errTooManyRedirects) {
		t.Fatalf("expected error to wrap errTooManyRedirects, got %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seenVia) != 2 {
		t.Fatalf("expected the policy to be invoked exactly twice (allow, then reject), got %d: %v", len(seenVia), seenVia)
	}
}

// TestSetCheckRedirectNilRestoresDefault verifies that a caller can clear a
// previously configured redirect policy by calling SetCheckRedirect(nil),
// restoring Go's default redirect-following behaviour.
func TestSetCheckRedirectNilRestoresDefault(t *testing.T) {
	var targetHits atomic.Int32

	target, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("landed"))
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer target.Close()

	origin, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL+"/landing")
		w.WriteHeader(http.StatusFound)
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer origin.Close()

	client := NewRequest(origin.URL, 5*time.Second, nil, nil)
	client.DisableRedirects()

	// Confirm redirects are indeed disabled first.
	resp, err := client.Get(context.Background(), "/start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected redirects to be disabled, got status %d", resp.StatusCode)
	}
	if targetHits.Load() != 0 {
		t.Fatalf("expected target not to be hit while redirects are disabled, got %d", targetHits.Load())
	}

	// Clear the policy — default behaviour must be restored.
	client.SetCheckRedirect(nil)
	if client.checkRedirect != nil {
		t.Fatal("expected checkRedirect to be nil after SetCheckRedirect(nil)")
	}
	if client.conn.CheckRedirect != nil {
		t.Fatal("expected conn.CheckRedirect to be nil after SetCheckRedirect(nil)")
	}

	resp, err = client.Get(context.Background(), "/start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected the default client to follow the redirect after clearing the policy, got %d", resp.StatusCode)
	}
	if targetHits.Load() != 1 {
		t.Fatalf("expected target to be hit exactly once after restoring default behaviour, got %d", targetHits.Load())
	}
}

// TestCloneCarriesRedirectPolicy verifies that Clone copies the configured
// redirect policy to the clone, and that the two remain independent after
// cloning.
func TestCloneCarriesRedirectPolicy(t *testing.T) {
	var targetHits atomic.Int32

	target, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer target.Close()

	origin, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL+"/landing")
		w.WriteHeader(http.StatusFound)
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer origin.Close()

	original := NewRequest(origin.URL, 5*time.Second, nil, nil)
	original.DisableRedirects()

	clone := original.Clone()

	resp, err := clone.Get(context.Background(), "/start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected clone to inherit the disabled-redirect policy, got status %d", resp.StatusCode)
	}
	if targetHits.Load() != 0 {
		t.Fatalf("clone must not follow the redirect, got %d hits on target", targetHits.Load())
	}

	// Independence: clearing the policy on the clone must not affect the original.
	clone.SetCheckRedirect(nil)
	if original.checkRedirect == nil {
		t.Fatal("clearing the clone's redirect policy must not affect the original")
	}
}

// TestRedirectPolicyPreservedAcrossSetProxyAndSetTLSConfig verifies that
// SetProxy and SetTLSConfig, which both rebuild the internal *http.Client,
// do not silently drop a previously configured redirect policy.
func TestRedirectPolicyPreservedAcrossSetProxyAndSetTLSConfig(t *testing.T) {
	client := NewRequest("https://example.com", 5*time.Second, nil, nil)
	client.DisableRedirects()

	proxyURL, err := url.Parse("http://proxy.example.com:8080")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	client.SetProxy(proxyURL)
	if client.conn.CheckRedirect == nil {
		t.Fatal("SetProxy must not reset a previously configured redirect policy")
	}
	if err := client.conn.CheckRedirect(&http.Request{}, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("expected the disable-redirects policy to survive SetProxy, got %v", err)
	}

	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	client.SetTLSConfig(tlsConfig)
	if client.conn.CheckRedirect == nil {
		t.Fatal("SetTLSConfig must not reset a previously configured redirect policy")
	}
	if err := client.conn.CheckRedirect(&http.Request{}, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("expected the disable-redirects policy to survive SetTLSConfig, got %v", err)
	}
}

// TestDisableRedirectsRetryOnConfigured3xx documents the actual resulting
// behaviour when a caller explicitly configures a 3xx status inside
// RetryConfig.RetryOn: the disabled-redirect response is retried like any
// other retryable status, while the redirect destination is still never
// contacted.
func TestDisableRedirectsRetryOnConfigured3xx(t *testing.T) {
	var originHits atomic.Int32
	var targetHits atomic.Int32

	target, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer target.Close()

	origin, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originHits.Add(1)
		w.Header().Set("Location", target.URL+"/landing")
		w.WriteHeader(http.StatusFound)
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer origin.Close()

	client := NewRequest(origin.URL, 5*time.Second, nil, nil)
	client.DisableRedirects()
	client.RetryConfig = RetryConfig{
		MaxAttempts: 3,
		RetryOn:     []int{http.StatusFound}, // 302 explicitly opted in as retryable
		BaseDelay:   1 * time.Millisecond,
		MaxDelay:    5 * time.Millisecond,
		Multiplier:  2.0,
	}

	resp, err := client.Get(context.Background(), "/start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected the final response to still be the unfollowed 302, got %d", resp.StatusCode)
	}
	if originHits.Load() != 3 {
		t.Fatalf("expected 3 attempts against origin because 302 was explicitly configured as retryable, got %d", originHits.Load())
	}
	if targetHits.Load() != 0 {
		t.Fatalf("redirect target must never be contacted even while retrying the disabled redirect, got %d", targetHits.Load())
	}
}

// TestDisableRedirectsDefaultRetryOnDoesNotRetry3xx verifies that a disabled
// redirect is NOT silently treated as retryable when RetryOn is left at its
// default — the default retry status list ([429, 500, 502, 503, 504]) does
// not include any 3xx code.
func TestDisableRedirectsDefaultRetryOnDoesNotRetry3xx(t *testing.T) {
	var originHits atomic.Int32

	origin, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originHits.Add(1)
		w.Header().Set("Location", "http://example.invalid/landing")
		w.WriteHeader(http.StatusFound)
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer origin.Close()

	client := NewRequest(origin.URL, 5*time.Second, nil, nil)
	client.DisableRedirects()
	client.RetryConfig = RetryConfig{
		MaxAttempts: 3, // RetryOn left nil -> defaults to [429, 500, 502, 503, 504]
		BaseDelay:   1 * time.Millisecond,
	}

	resp, err := client.Get(context.Background(), "/start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if originHits.Load() != 1 {
		t.Fatalf("expected exactly 1 attempt: default RetryOn does not include 3xx, got %d", originHits.Load())
	}
}

// TestDisableRedirectsCircuitBreakerNotTrippedByDefault verifies that a
// disabled-redirect response does not count as a circuit-breaker failure
// under the default FailOn policy, so the circuit never opens.
func TestDisableRedirectsCircuitBreakerNotTrippedByDefault(t *testing.T) {
	origin, ok := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://example.invalid/landing")
		w.WriteHeader(http.StatusFound)
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer origin.Close()

	client := NewRequest(origin.URL, 5*time.Second, nil, nil)
	client.DisableRedirects()
	client.CircuitBreaker = CircuitBreakerConfig{
		Enabled:      true,
		MaxFailures:  2,
		ResetTimeout: 10 * time.Second,
		// FailOn left nil -> defaults to the same list as RetryConfig.RetryOn's default.
	}

	for i := range 5 {
		resp, err := client.Get(context.Background(), "/start")
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if resp.StatusCode != http.StatusFound {
			t.Fatalf("call %d: expected 302, got %d", i, resp.StatusCode)
		}
	}
}

// newTLSTestServer starts an httptest TLS server bound to 127.0.0.1 (IPv4).
// Returns (server, true) on success, or (nil, false) if the environment does
// not allow TCP listeners (e.g. a restricted sandbox), matching the
// newTestServer skip convention used throughout this file.
func newTLSTestServer(handler http.Handler) (*httptest.Server, bool) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, false
	}
	srv := httptest.NewUnstartedServer(handler)
	srv.Listener = l
	srv.StartTLS()
	return srv, true
}

// trustedTLSConfig returns a *tls.Config that trusts srv's certificate via a
// dedicated CertPool, so TLS ownership tests exercise real certificate
// verification instead of InsecureSkipVerify.
func trustedTLSConfig(srv *httptest.Server) *tls.Config {
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	return &tls.Config{RootCAs: pool}
}

// TestNewRequestSnapshotsCallerTLSConfig proves that NewRequest takes a
// private snapshot of the caller-supplied *tls.Config instead of retaining
// the caller-owned pointer: mutating the original after construction must
// not alter the Request, and the Request must not hold the same pointer.
func TestNewRequestSnapshotsCallerTLSConfig(t *testing.T) {
	original := &tls.Config{ServerName: "caller-owned.example.com"}

	client := NewRequest("https://example.com", 5*time.Second, original, nil)

	// Mutate the caller's original config after construction.
	original.ServerName = "mutated-after-construction.example.com"
	original.InsecureSkipVerify = true

	got := client.GetTLSConfig()
	if got == original {
		t.Fatal("NewRequest must not retain the caller's *tls.Config pointer")
	}
	if got.ServerName != "caller-owned.example.com" {
		t.Fatalf("Request's TLS config was affected by a later mutation to the caller's config: got ServerName %q", got.ServerName)
	}
	if got.InsecureSkipVerify {
		t.Fatal("Request's TLS config was affected by a later mutation to the caller's config")
	}
}

// TestSetTLSConfigSnapshotsCallerConfig proves that SetTLSConfig also takes a
// private snapshot rather than retaining the caller-owned pointer.
func TestSetTLSConfigSnapshotsCallerConfig(t *testing.T) {
	client := NewRequest("https://example.com", 5*time.Second, nil, nil)

	original := &tls.Config{ServerName: "caller-owned.example.com"}
	client.SetTLSConfig(original)

	original.ServerName = "mutated-after-set.example.com"

	got := client.GetTLSConfig()
	if got == original {
		t.Fatal("SetTLSConfig must not retain the caller's *tls.Config pointer")
	}
	if got.ServerName != "caller-owned.example.com" {
		t.Fatalf("Request's TLS config was affected by a later mutation to the caller's config: got ServerName %q", got.ServerName)
	}
}

// TestSetTLSConfigNilDoesNotPanic verifies that passing nil to SetTLSConfig
// resets the transport to Go's default TLS behaviour instead of panicking.
func TestSetTLSConfigNilDoesNotPanic(t *testing.T) {
	client := NewRequest("https://example.com", 5*time.Second, &tls.Config{InsecureSkipVerify: true}, nil)

	client.SetTLSConfig(nil)

	if got := client.GetTLSConfig(); got != nil {
		t.Fatalf("expected nil TLS config after SetTLSConfig(nil), got %+v", got)
	}
}

// TestCloneTLSConfigIndependent verifies that Clone gives the clone an
// independent TLS configuration snapshot: neither shares mutable TLS state
// with the original, in either direction.
func TestCloneTLSConfigIndependent(t *testing.T) {
	shared := &tls.Config{ServerName: "shared.example.com"}
	original := NewRequest("https://example.com", 5*time.Second, shared, nil)
	clone := original.Clone()

	if clone.GetTLSConfig() == original.GetTLSConfig() {
		t.Fatal("Clone must have an independent TLS config from the original")
	}

	clone.SetTLSConfig(&tls.Config{ServerName: "clone-only.example.com"})
	if original.GetTLSConfig().ServerName == "clone-only.example.com" {
		t.Fatal("mutating the clone's TLS config must not affect the original")
	}

	original.SetTLSConfig(&tls.Config{ServerName: "original-only.example.com"})
	if clone.GetTLSConfig().ServerName == "original-only.example.com" {
		t.Fatal("mutating the original's TLS config must not affect the clone")
	}
}

// TestSetProxyDoesNotAlterTLSConfig verifies that SetProxy, which rebuilds
// the internal *http.Client, does not replace or clear the Request's TLS
// configuration.
func TestSetProxyDoesNotAlterTLSConfig(t *testing.T) {
	tlsConfig := &tls.Config{ServerName: "example.com"}
	client := NewRequest("https://example.com", 5*time.Second, tlsConfig, nil)
	before := client.GetTLSConfig()

	proxyURL, err := url.Parse("http://proxy.example.com:8080")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	client.SetProxy(proxyURL)

	after := client.GetTLSConfig()
	if before != after {
		t.Fatal("SetProxy must not replace or clear the Request's TLS configuration")
	}
}

// TestSetTLSConfigConcurrentRequestsIndependentFromSharedInput verifies that
// when multiple independently constructed Requests are configured
// concurrently via SetTLSConfig using the same caller-owned *tls.Config
// pointer, each Request ends up with its own independent, private TLS
// configuration rather than sharing one.
func TestSetTLSConfigConcurrentRequestsIndependentFromSharedInput(t *testing.T) {
	shared := &tls.Config{ServerName: "shared.example.com"}

	const n = 8
	requests := make([]*Request, n)
	for i := range requests {
		requests[i] = NewRequest("https://example.com", 5*time.Second, nil, nil)
	}

	var wg sync.WaitGroup
	for _, r := range requests {
		wg.Add(1)
		go func(req *Request) {
			defer wg.Done()
			req.SetTLSConfig(shared)
		}(r)
	}
	wg.Wait()

	seen := make(map[*tls.Config]bool, n)
	for _, r := range requests {
		cfg := r.GetTLSConfig()
		if cfg == shared {
			t.Fatal("SetTLSConfig must not retain the caller's *tls.Config pointer")
		}
		if seen[cfg] {
			t.Fatal("two Requests must not share the same cloned *tls.Config pointer")
		}
		seen[cfg] = true
	}
}

// TestConcurrentRequestsSharedTLSConfigNoRace is the required race regression
// test for the TLS ownership fix. It reproduces the usage pattern that
// exposed the original defect: multiple independently constructed
// httpclient.Request instances built from one shared caller-owned
// *tls.Config, dispatching real HTTPS requests concurrently against a local
// httptest TLS server, exercising HTTP transport/TLS initialization for the
// first time on each Request concurrently.
//
// Before the fix, this races on the shared *tls.Config:
// net/http.(*Transport).onceSetNextProtoDefaults writes
// TLSClientConfig.NextProtos directly on the shared object (see the
// "Transport doesn't [clone]" comment in the net/http source), while
// crypto/tls concurrently reads the same config during the handshake. All
// requests must succeed and go test -race must report no race.
func TestConcurrentRequestsSharedTLSConfigNoRace(t *testing.T) {
	srv, ok := newTLSTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	if !ok {
		t.Skip("TCP listener not available in this environment")
	}
	defer srv.Close()

	sharedTLSConfig := trustedTLSConfig(srv)

	const numClients = 16
	clients := make([]*Request, numClients)
	for i := range clients {
		clients[i] = NewRequest(srv.URL, 5*time.Second, sharedTLSConfig, nil)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, numClients)
	for _, client := range clients {
		wg.Add(1)
		go func(c *Request) {
			defer wg.Done()
			resp, err := c.Get(context.Background(), "/ping")
			if err != nil {
				errCh <- err
				return
			}
			if resp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("unexpected status %d", resp.StatusCode)
			}
		}(client)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent request failed: %v", err)
	}
}
