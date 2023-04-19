package httpclient

import (
	"encoding/json"
	"strings"
)

// GetHeader - returns value of a given header
func (response *Response) GetHeader(key string) (value []string) {

	var hkey string
	for hkey, value = range response.headers {
		if strings.EqualFold(hkey, key) {
			return // we found what we wanted, return
		}
	}

	return nil // if we don't find what we want, just return nil
}

// GetContentType - returns the response content type
func (response *Response) GetContentType() (contentType *ContentType) {

	values := response.GetHeader("Content-Type")
	if len(values) == 0 {
		return // there is no content type
	}

	splitStr := strings.Split(values[0], ";")

	contentType = new(ContentType)
	contentType.Media = splitStr[0]

	splitLen := len(splitStr) - 1 // index starts from 0
	for i := 1; i <= splitLen; i++ {
		remain := strings.Split(splitStr[i], "=")

		// proceed only if we have EXACTLY 2 values
		if len(remain) == 2 {
			if strings.HasPrefix(strings.ToLower(remain[0]), "charset") {
				contentType.Charset = remain[1]
			} else if strings.HasPrefix(strings.ToLower(remain[0]), "boundary") {
				contentType.Boundary = remain[1]
			}
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
