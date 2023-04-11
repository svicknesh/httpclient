package httpclient

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
	return []byte(p.String()), nil
}
