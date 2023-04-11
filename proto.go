package httpclient

import (
	"encoding/json"
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
	protostr := string(protobytes)
	protostr = strings.ReplaceAll(protostr, "\"", "")

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
