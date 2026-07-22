package hc

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
)

func TestCheckReportsBadHTTPStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	checkID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	check, err := NewUUIDCheck(checkID, WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewUUIDCheck() error = %v", err)
	}

	err = check.Success(t.Context())
	var statusErr BadStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("Success() error = %T %[1]v, want BadStatusError", err)
	}
	if statusErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("BadStatusError.StatusCode = %d, want %d", statusErr.StatusCode, http.StatusInternalServerError)
	}
	if got, want := statusErr.Req.URL.Path, "/"+checkID.String(); got != want {
		t.Fatalf("BadStatusError.Req.URL.Path = %q, want %q", got, want)
	}
}

func TestCheckReportsRequestFailureCause(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	serverURL := server.URL
	server.Close()

	checkID := uuid.MustParse("00000000-0000-4000-8000-000000000002")
	check, err := NewUUIDCheck(checkID, WithBaseURL(serverURL))
	if err != nil {
		t.Fatalf("NewUUIDCheck() error = %v", err)
	}

	err = check.Success(t.Context())
	var requestErr RequestFailedError
	if !errors.As(err, &requestErr) {
		t.Fatalf("Success() error = %T %[1]v, want RequestFailedError", err)
	}
	if requestErr.Err == nil {
		t.Fatal("RequestFailedError.Err = nil, want underlying request error")
	}
}

func TestCheckAddsRunIDToExistingQuery(t *testing.T) {
	t.Parallel()

	requestQuery := make(chan url.Values, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestQuery <- r.URL.Query()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	checkID := uuid.MustParse("00000000-0000-4000-8000-000000000003")
	runID := uuid.MustParse("00000000-0000-4000-8000-000000000004")
	check, err := NewUUIDCheck(checkID, WithBaseURL(server.URL+"?existing=true"))
	if err != nil {
		t.Fatalf("NewUUIDCheck() error = %v", err)
	}

	if err := check.Success(t.Context(), WithRunID(runID)); err != nil {
		t.Fatalf("Success() error = %v", err)
	}

	got := <-requestQuery
	if got.Get("existing") != "true" {
		t.Fatalf("query existing = %q, want true", got.Get("existing"))
	}
	if got.Get("rid") != runID.String() {
		t.Fatalf("query rid = %q, want %q", got.Get("rid"), runID.String())
	}
}

func TestCheckMethodsUseExpectedPathsAndBodies(t *testing.T) {
	t.Parallel()

	type requestRecord struct {
		path string
		body string
	}

	records := make(chan requestRecord, 5)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("io.ReadAll() error = %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		records <- requestRecord{path: r.URL.Path, body: string(body)}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	checkID := uuid.MustParse("00000000-0000-4000-8000-000000000005")
	check, err := NewUUIDCheck(checkID, WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewUUIDCheck() error = %v", err)
	}

	ctx := t.Context()
	calls := []struct {
		name string
		call func() error
		path string
		body string
	}{
		{name: "success", call: func() error { return check.Success(ctx) }, path: "/" + checkID.String()},
		{name: "start", call: func() error { return check.Start(ctx) }, path: "/" + checkID.String() + "/start"},
		{name: "failure", call: func() error { return check.Failure(ctx) }, path: "/" + checkID.String() + "/fail"},
		{name: "complete status", call: func() error { return check.CompleteStatus(ctx, 7) }, path: "/" + checkID.String() + "/7"},
		{name: "log", call: func() error { return check.Log(ctx, "diagnostics") }, path: "/" + checkID.String() + "/log", body: "diagnostics"},
	}

	for _, tc := range calls {
		if err := tc.call(); err != nil {
			t.Fatalf("%s call error = %v", tc.name, err)
		}
		record := <-records
		if record.path != tc.path {
			t.Fatalf("%s path = %q, want %q", tc.name, record.path, tc.path)
		}
		if record.body != tc.body {
			t.Fatalf("%s body = %q, want %q", tc.name, record.body, tc.body)
		}
	}
}

func TestBadStatusErrorMessageIncludesRequestPath(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"https://example.com/check/start?rid=abc",
		http.NoBody,
	)
	req.RequestURI = ""
	err := BadStatusError{Req: req, StatusCode: http.StatusNotFound}

	got := err.Error()
	want := fmt.Sprintf("bad status code for %q: %d", "POST /check/start?rid=abc", http.StatusNotFound)
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestNewSlugCheckUsesInjectedClient(t *testing.T) {
	t.Parallel()

	requestURL := make(chan string, 1)
	client := &http.Client{Transport: requestRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requestURL <- req.URL.String()
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})}
	check, err := NewSlugCheck(
		"pingkey",
		"myslug",
		WithBaseURL("https://hc-ping.invalid"),
		WithHTTPClient(client),
	)
	if err != nil {
		t.Fatalf("NewSlugCheck() error = %v", err)
	}
	if err := check.Success(t.Context()); err != nil {
		t.Fatalf("Success() error = %v", err)
	}
	if got, want := <-requestURL, "https://hc-ping.invalid/pingkey/myslug"; got != want {
		t.Fatalf("request URL = %q, want %q", got, want)
	}
}

type requestRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f requestRoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
