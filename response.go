package httpclient

import (
	"encoding/json"
	"net/http"
	"strings"
)

// GetHeader - returns value of a given header
func (response *Response) GetHeader(key string) (value []string) {

	if hdrs, ok := response.headers[http.CanonicalHeaderKey(key)]; ok {
		return hdrs
	}

	// Fallback: iterate with a case-insensitive check.
	for k, v := range response.headers {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return nil // if we don't find what we want, just return nil
}

// GetContentType - returns the response content type
func (response *Response) GetContentType() (ct *ContentType) {

	values := response.GetHeader("Content-Type")
	if len(values) == 0 {
		return nil
	}

	// Split the header on ";" to separate the media type from parameters.
	parts := strings.Split(values[0], ";")
	ct = &ContentType{
		Media: strings.TrimSpace(parts[0]),
	}

	// Process additional parameters using SplitN to minimize allocations.
	for i := 1; i < len(parts); i++ {
		param := strings.TrimSpace(parts[i])
		if param == "" {
			continue
		}
		kv := strings.SplitN(param, "=", 2)
		if len(kv) != 2 {
			continue
		}
		keyParam := strings.ToLower(strings.TrimSpace(kv[0]))
		valParam := strings.TrimSpace(kv[1])
		switch keyParam {
		case "charset":
			ct.Charset = valParam
		case "boundary":
			ct.Boundary = valParam
		}
	}

	return
}

// IsJSON - determine if the return value is of type JSON
func (response *Response) IsJSON() (isjson bool) {
	ct := response.GetContentType()
	if nil == ct {
		return
	}

	return strings.EqualFold(ct.Media, "application/json")
}

// ToJSON - convert HTTP client response to JSON
func (response *Response) ToJSON(output interface{}) (err error) {
	return json.NewDecoder(&response.Buffer).Decode(output)
}
