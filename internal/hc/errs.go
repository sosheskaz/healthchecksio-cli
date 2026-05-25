package hc

import (
	"fmt"
	"net/http"
)

func reqHeader(req *http.Request) string {
	if req == nil {
		return "<nil request>"
	}

	requestURI := req.RequestURI
	if requestURI == "" && req.URL != nil {
		requestURI = req.URL.RequestURI()
	}

	return req.Method + " " + requestURI
}

// BadStatusError is returned when a request returns a bad status code.
type BadStatusError struct {
	Req        *http.Request
	StatusCode int
}

func (e BadStatusError) Error() string {
	reqSubject := reqHeader(e.Req)
	return fmt.Sprintf("bad status code for %q: %d", reqSubject, e.StatusCode)
}

// RequestFailedError is returned when a request fails to complete.
type RequestFailedError struct {
	Req *http.Request
	Err error
}

func (e RequestFailedError) Error() string {
	reqSubject := reqHeader(e.Req)
	return fmt.Sprintf("request failed for %q: %+v", reqSubject, e.Err)
}

// Unwrap returns the underlying request error.
func (e RequestFailedError) Unwrap() error {
	return e.Err
}

// BadConfigError is returned when the configuration is invalid.
type BadConfigError struct {
	Message string
}

func (e BadConfigError) Error() string {
	return e.Message
}
