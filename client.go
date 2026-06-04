package httpclient

import (
	"crypto/tls"
	"encoding/json"
)

// NewTLSConfig - creates a new TLS configuration with secure defaults.
func NewTLSConfig() (tlsConfig *tls.Config) {
	return &tls.Config{}
}

// NewInsecureTLSConfig - creates a TLS configuration with certificate verification disabled.
// WARNING: do not use in production; connections are vulnerable to man-in-the-middle attacks.
func NewInsecureTLSConfig() (tlsConfig *tls.Config) {
	return &tls.Config{InsecureSkipVerify: true} //nolint:gosec
}

func (h *Headers) Set(header, value string) {
	*h = append(*h, Header{Key: header, Value: value})
}

func (h Headers) String() (str string) {
	bytes, _ := json.Marshal(h)
	return string(bytes)
}
