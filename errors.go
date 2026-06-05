package httpclient

import "errors"

// ErrCircuitOpen is returned by connect when the circuit breaker is open and
// the reset timeout has not yet elapsed.
var ErrCircuitOpen = errors.New("httpclient: circuit breaker is open")
