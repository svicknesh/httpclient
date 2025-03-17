package httpclient

import (
	"encoding/json"
	"strconv"
	"strings"
)

type Protocol uint8

const (
	// ProtocolHTTP1 - Use HTTP1.1 for communication
	ProtocolHTTP1 Protocol = 1 + iota

	// ProtocolHTTP2 - Use HTTP2 for communication
	ProtocolHTTP2
)

func (p Protocol) String() (str string) {
	protoNames := []string{"unknown", "http", "http2"}
	if int(p) > len(protoNames)-1 {
		return "unknown"
	}
	return protoNames[p]
}

func (p Protocol) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.String())
}

func (p *Protocol) UnmarshalJSON(protobytes []byte) (err error) {

	// Use strconv.Unquote to remove surrounding quotes.
	protostr, err := strconv.Unquote(string(protobytes))
	if err != nil {
		// Fallback to trimming quotes manually.
		protostr = strings.Trim(string(protobytes), "\"")
	}

	switch protostr {
	case "http":
		*p = ProtocolHTTP1
	case "http2":
		*p = ProtocolHTTP2
	default:
		*p = 0 // unknown protocol
	}

	return
}
