# Golang HTTP Client library

## Initialize new HTTP/1.1 or HTTP/2 client with TLS support

```go
tlsConfig := &tls.Config{InsecureSkipVerify: true}
client := NewRequest("https://httpclienttest.free.beeceptor.com", 10, tlsConfig, Headers{Header{Key: "Content-type", Value: "application/json"}})

client.SetHeader("my-custom-header", "cool value yo!")
```


## Make a GET Request

```go
response, err := client.Get("/users")
if nil != err {
    log.Println(err)
    return
}

log.Println(response.StatusCode)

log.Println("Media type is " + response.GetContentType().Media)
log.Printf("Is JSON response: %t", response.IsJSON())

if response.IsJSON() {
    type User struct {
        ID       int    `json:"id"`
        Username string `json:"username"`
    }

    var users []User
    DecodeJSON(response, &users)
    response.ToJSON(&users)
    fmt.Println(users)
}
```


## Make a POST Request

```go

payload, _ := json.Marshal(values) // where values is a JSON structure
httpResponse, err := client.Post("/user", bytes.NewReader(payload)) // for POST and PUT, the payload is expected to be an io.Reader
if nil != err {
    return
}
```


## Make a PUT Request

```go

payload, _ := json.Marshal(values) // where values is a JSON structure
httpResponse, err := client.Put("/user/1", bytes.NewReader(payload)) // for POST and PUT, the payload is expected to be an io.Reader
if nil != err {
    return
}
```


## Make a DELETE Request

```go

httpResponse, err := client.Delete("/user/1") // for POST and PUT, the payload is expected to be an io.Reader
if nil != err {
    return
}
```


## Other options

```go
//override tls configuration with a new one
tlsConfig := &tls.Config{InsecureSkipVerify: true}

tlsConfig.Certificates = []tls.Certificate{clientCert}
tlsConfig.Renegotiation = tls.RenegotiateOnceAsClient

client.SetTLSConfig(tlsConfig) // sets custom TLS configuration

client.EnableSuffix(false) // temporarily disable usage of suffix
response, err := client.Get("/healthcheck")
client.EnableSuffix(false) // enable usage of suffix
```

TLS configurations supplied to `NewRequest` or `SetTLSConfig` are snapshotted.
Callers may reuse or mutate their original `tls.Config` after configuration
without affecting the Request — this also means independent Requests built
from the same shared `*tls.Config` are safe to use concurrently.


## Redirect control

By default, a `Request` created via `NewRequest` follows HTTP redirects using
Go's standard `net/http` behaviour — this default is unchanged unless you opt
in to one of the methods below.

Go's default redirect handling can forward `Authorization` across same-host
redirects, HTTPS→HTTP downgrades on the same host, and parent-domain →
subdomain redirects. It does **not** know about arbitrary custom headers such
as `X-Api-Key` — those are never stripped, regardless of where the redirect
points. Do not rely on Go's default behaviour to protect custom auth headers.

### DisableRedirects

Use `DisableRedirects` when a request carries credentials that must never be
sent to a redirect destination — for example, an authenticated webhook
delivery. The redirect response (301, 302, 303, 307, 308, or any other 3xx)
is returned to the caller as an ordinary `Response` instead of being
followed, so the redirect destination is never contacted:

```go
client := httpclient.NewRequest(
    "https://api.example.com",
    15*time.Second,
    tlsConfig,
    headers,
)

client.DisableRedirects()

resp, err := client.Post(ctx, "/webhook", body)
if err != nil {
    return err
}
if resp.StatusCode >= 300 && resp.StatusCode < 400 {
    // Treat the redirect as a delivery failure; credentials were never
    // forwarded to resp.GetHeader("Location").
}
```

### SetCheckRedirect

For finer control, install a custom policy with the same semantics as
`http.Client.CheckRedirect`: it is called before each redirect is followed,
receives the pending request and the requests already made (oldest first, in
`via`), and can allow, reject, or otherwise inspect the redirect chain.
Passing `nil` restores Go's default redirect behaviour.

```go
client.SetCheckRedirect(func(req *http.Request, via []*http.Request) error {
    if len(via) >= 3 {
        return errors.New("stopped after 3 redirects")
    }
    return nil
})
```

`DisableRedirects` is equivalent to calling `SetCheckRedirect` with a policy
that always returns `http.ErrUseLastResponse`.

### Concurrency and lifecycle

`SetCheckRedirect`/`DisableRedirects`, like the rest of `Request`'s
configuration methods (`SetHeader`, `SetRetry`-style fields, etc.), are
configuration-time settings. Configure a `Request` before using it
concurrently; do not mutate its configuration while requests issued from it
may still be in flight. `Request` does not add locking around these methods.

### Clone behaviour

`Clone()` copies the configured redirect policy to the clone, applied
independently to the clone's own internal HTTP client — changing the
clone's policy afterwards does not affect the original, and vice versa.

### Interaction with retry and the circuit breaker

A redirect response returned because redirects are disabled is treated as an
ordinary HTTP response, not a transport error — it flows through the
existing `RetryConfig`/`CircuitBreaker` logic unchanged. By default, neither
`RetryConfig.RetryOn` nor `CircuitBreaker.FailOn` includes any 3xx status, so
a disabled redirect is not retried and does not count as a circuit-breaker
failure unless you explicitly add a 3xx status to one of those lists.
