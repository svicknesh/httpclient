# Golang HTTP Client library

## Initialize new HTTP/1.1 client with TLS support

```go
tlsConfig := &tls.Config{InsecureSkipVerify: true}
client := NewRequest("https://httpclienttest.free.beeceptor.com", 10, tlsConfig, Headers{Header{Key: "Content-type", Value: "application/json"}})

client.SetHeader("my-custom-header", "cool value yo!")

```

## Make a GET request

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

## Make a POST REQUEST

```go

payload, _ := json.Marshal(values) // where values is a JSON structure
httpResponse, err := client.Put("/remote", bytes.NewReader(payload))
if nil != err {
    return
}


```
