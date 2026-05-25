package hc

import (
	"fmt"
	"net/http"
)

func reqHeader(req *http.Request) string {
	if req == nil {
		return "<nil request>"
	}
	return req.Method + " " + req.RequestURI
}

// BadStatusError is returned when a request returns a bad status code.
type BadStatusError struct {
	Req        *http.Request
	StatusCode int
}

func (e BadStatusError) Error() string {
	reqSubject := reqHeader(e.Req)
	return fmt.Sprintf("Bad status code for %q: %d", reqSubject, e.StatusCode)
}

// RequestFailedError is returned when a request fails to complete.
type RequestFailedError struct {
	Req *http.Request
	Err error
}

func (e RequestFailedError) Error() string {
	reqSubject := reqHeader(e.Req)
	return fmt.Sprintf("Request failed for %q: %+v", reqSubject, e.Err)
}

// BadConfigError is returned when the configuration is invalid.
type BadConfigError struct {
	Message string
}

func (e BadConfigError) Error() string {
	return e.Message
}
