package httpclient

import (
	"bytes"
	"net/http"
)

// Request - client connection for  requests
type Request struct {
	Address string
	Suffix  string // some applications will have a default suffix, this reduces the typing or configuration
	conn    *http.Client
	headers http.Header
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
