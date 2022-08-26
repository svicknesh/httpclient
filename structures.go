package httpclient

import (
	"bytes"
	"net/http"
)

// Request - client connection for  requests
type Request struct {
	UseTLS bool
	//Protocol string
	Address string
	Port    int
	conn    *http.Client
	headers http.Header
	server  string
	//remoteAddress string
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
	//Bytes      []byte
}

// ContentType - response content type header
type ContentType struct {
	Media    string
	Charset  string
	Boundary string
}
