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
