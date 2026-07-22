package httpclient

import (
	"context"
	"crypto/tls"
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
