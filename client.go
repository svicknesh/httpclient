package httpclient

import (
	"crypto/tls"
	"encoding/json"
)

// NewTLSConfig - creates a new TLS cofiguration with the option `InsecureSkipVerify` set to true.
func NewTLSConfig() (tlsConfig *tls.Config) {
	return &tls.Config{InsecureSkipVerify: true}
}

func (h *Headers) Set(header, value string) {
	*h = append(*h, Header{Key: header, Value: value})
}

func (h Headers) String() (str string) {
	bytes, _ := json.Marshal(h)
	return string(bytes)
}
