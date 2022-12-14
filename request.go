package httpclient

import (
	"crypto/tls"
	"io"
	"net/http"
	"time"
)

// NewRequest - create a new instance of Request
func NewRequest(address string, timeout int, tlsConfig *tls.Config, headers Headers) (request *Request) {

	request = new(Request)

	//request.Protocol = protocol
	request.Address = address

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	transport.MaxIdleConns = 100
	transport.MaxConnsPerHost = 100
	transport.MaxIdleConnsPerHost = 100

	request.conn = &http.Client{Transport: transport, Timeout: time.Duration(timeout) * time.Second}

	request.headers = make(http.Header)

	for _, header := range headers {
		request.headers.Set(header.Key, header.Value)
	}

	return
}

// Get - connect to the service with the given data using the  GET request
func (request *Request) Get(endpoint string) (httpResponse *Response, err error) {
	return request.connect("GET", endpoint, nil)
}

// Post - connect to the service with the given data using the  POST request
func (request *Request) Post(endpoint string, payload io.Reader) (httpResponse *Response, err error) {
	return request.connect("POST", endpoint, payload)
}

// Put - connect to the service with the given data using the  PUT request
func (request *Request) Put(endpoint string, payload io.Reader) (httpResponse *Response, err error) {
	return request.connect("PUT", endpoint, payload)
}

// Delete - connect to the service with the given data using the  DELETE request
func (request *Request) Delete(endpoint string) (httpResponse *Response, err error) {
	return request.connect("DELETE", endpoint, nil)
}

// SetHeader - sets additional header for the client
func (request *Request) SetHeader(key, value string) {
	request.headers.Set(key, value)
}

// GetHeader - gets a header specified by the key
func (request *Request) GetHeader(key string) (value string) {
	return request.headers.Get(key)
}

// SetSuffix - sets a base suffix for all endpoint operations
func (request *Request) SetSuffix(suffix string) {
	request.Suffix = suffix
}

// connect - execute the connection
func (request *Request) connect(method, endpoint string, payload io.Reader) (response *Response, err error) {
	//address := fmt.Sprintf("%s%s", request.Address, endpoint) // don't enclose address in [] otherwise domain names won't work
	address := request.Address + request.Suffix + endpoint // don't enclose address in [] otherwise domain names won't work

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

	/*
		b, err := io.ReadAll(httpResponse.Body)
		if err != nil {
			return
		}
		response.Buffer.Write(b)
	*/

	response.headers = httpResponse.Header // save the response headers for later use
	//fmt.Println(response.headers)

	return
}
