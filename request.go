package httpclient

import (
	"crypto/tls"
	"io"
	"net/http"
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

	request.conn = &http.Client{Transport: request.transport, Timeout: timeout * time.Second}

	request.headers = make(http.Header)

	for _, header := range headers {
		request.headers.Set(header.Key, header.Value)
	}

	return
}

// Get - connect to the service with the given data using the  GET HTTP request verb
func (request *Request) Get(endpoint string) (httpResponse *Response, err error) {
	return request.connect("GET", endpoint, nil)
}

// Post - connect to the service with the given data using the  POST HTTP request verb
func (request *Request) Post(endpoint string, payload io.Reader) (httpResponse *Response, err error) {
	return request.connect("POST", endpoint, payload)
}

// Put - connect to the service with the given data using the  PUT HTTP request verb
func (request *Request) Put(endpoint string, payload io.Reader) (httpResponse *Response, err error) {
	return request.connect("PUT", endpoint, payload)
}

// Patch - connect to the service with the given data using the  PATCH HTTP request verb
func (request *Request) Patch(endpoint string, payload io.Reader) (httpResponse *Response, err error) {
	return request.connect("PATCH", endpoint, payload)
}

// Delete - connect to the service with the given data using the  DELETE HTTP request verb
func (request *Request) Delete(endpoint string) (httpResponse *Response, err error) {
	return request.connect("DELETE", endpoint, nil)
}

// Options - connect to the service with the given data using the  OPTIONS HTTP request verb
func (request *Request) Options(endpoint string) (httpResponse *Response, err error) {
	return request.connect("OPTIONS", endpoint, nil)
}

// Custom - connect to the service with the given data using a custom HTTP request verb
func (request *Request) Custom(httpVerb, endpoint string, payload io.Reader) (httpResponse *Response, err error) {
	return request.connect(httpVerb, endpoint, payload)
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

// SetTLSConfig - overrides existing TLS configuration with a new one, HTTP/2 does not support renegotiation so we need to downgrade it to HTTP/1.1
func (request *Request) SetTLSConfig(tlsConfig *tls.Config) {
	tlsConfig.Renegotiation = tls.RenegotiateOnceAsClient // we request renegotiation by the server to use the latest TLS configuration

	tr := request.transport

	tr.TLSClientConfig = tlsConfig
	tr.CloseIdleConnections() // tear down old TLS config and use new one for any already‑open or idle connections

	// disable HTTP/2 if really need renegotiation
	//tr.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}

	request.conn = &http.Client{Transport: tr, Timeout: request.timeout * time.Second} // recreate the connection using the new TLS configuration
}

// connect - execute the connection
func (request *Request) connect(method, endpoint string, payload io.Reader) (response *Response, err error) {

	// Build the full URL.
	var sb strings.Builder
	sb.WriteString(request.Address)
	if request.suffixEnabled && request.Suffix != "" {
		sb.WriteString(request.Suffix)
	}
	// Trim any leading '/' from endpoint.
	sb.WriteString(strings.TrimLeft(endpoint, "/"))
	address := sb.String() // don't enclose address in [] otherwise domain names won't work, remove any leading '/' from the endpoint
	//fmt.Println(address)

	httpRequest, err := http.NewRequest(method, address, payload)
	if err != nil {
		return
	}

	httpRequest.Header = request.headers

	httpResponse, err := request.conn.Do(httpRequest)
	if err != nil {
		return // return the error
	}
	defer httpResponse.Body.Close()

	response = new(Response)
	response.StatusCode = httpResponse.StatusCode

	_, err = io.Copy(&response.Buffer, httpResponse.Body)
	if nil != err {
		return
	}

	response.headers = httpResponse.Header // save the response headers for later use
	//fmt.Println(response.headers)

	response.TLS = httpResponse.TLS // saves the response TLS

	return
}
