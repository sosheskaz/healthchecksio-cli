package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sosheskaz/healthchecksio-cli/internal/hc"
)

const execCommandHelperEnv = "HEALTHCHECKSIO_CLI_EXEC_HELPER"

func TestExecCommandReportsSubcommandExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		helperMode string
	}{
		{name: "check flag", helperMode: "exec"},
		{name: "environment check ID", helperMode: "exec-env"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testExecCommandReportsSubcommandExitCode(t, tc.helperMode)
		})
	}
}

func testExecCommandReportsSubcommandExitCode(t *testing.T, helperMode string) {
	t.Helper()

	checkID := uuid.MustParse("00000000-0000-4000-8000-000000000007")
	var (
		mu           sync.Mutex
		requestPaths []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestPaths = append(requestPaths, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	helper := exec.CommandContext(
		t.Context(),
		os.Args[0],
		"-test.run=TestExecCommandHelper",
		"--",
		helperMode,
		server.URL,
		checkID.String(),
	)
	helper.Env = append(os.Environ(), execCommandHelperEnv+"=1")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	helper.Stdout = &stdout
	helper.Stderr = &stderr

	err := helper.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("helper error = %T %[1]v, want exec.ExitError; stdout: %s; stderr: %s", err, stdout.String(), stderr.String())
	}
	if got, want := exitErr.ExitCode(), 7; got != want {
		t.Fatalf("helper exit code = %d, want %d; stdout: %s; stderr: %s", got, want, stdout.String(), stderr.String())
	}

	mu.Lock()
	gotPaths := append([]string(nil), requestPaths...)
	mu.Unlock()
	wantPaths := []string{"/" + checkID.String() + "/start", "/" + checkID.String() + "/7"}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("request paths = %v, want %v", gotPaths, wantPaths)
	}
}

func TestExecCommandNamesInvalidEnvironmentCheckIDWithoutPanicking(t *testing.T) {
	const invalidCheckID = "sensitive-invalid-check-id"
	t.Setenv(envCheckID, invalidCheckID)

	factoryCalled := false
	cmd := rootCommandWithClientFactory(func(hc.RetryConfig) (*http.Client, error) {
		factoryCalled = true
		return http.DefaultClient, nil
	})
	cmd.SetArgs([]string{"exec", "--", os.Args[0], "-test.run=^$"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Errorf("Execute() panicked for invalid %s: %v", envCheckID, recovered)
		}
	}()

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !errors.Is(err, errInvalidEnvironmentValue) {
		t.Errorf("Execute() error = %v, want invalid environment value", err)
	}
	if !strings.Contains(err.Error(), envCheckID) {
		t.Errorf("Execute() error = %q, want environment name %q", err, envCheckID)
	}
	if strings.Contains(err.Error(), invalidCheckID) {
		t.Errorf("Execute() error exposed environment value: %v", err)
	}
	if factoryCalled {
		t.Fatal("client factory called for invalid check ID")
	}
}

func TestExecCheckFlagOverridesEnvironmentCheckID(t *testing.T) {
	checkID := uuid.MustParse("00000000-0000-4000-8000-000000000020")
	t.Setenv(envCheckID, "invalid-environment-check-id")

	requestPaths := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPaths <- r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	cmd := rootCommandWithClientFactory(routeHealthchecksTo(t, server.URL))
	cmd.SetArgs([]string{
		"exec",
		"--check", checkID.String(),
		"--",
		os.Args[0],
		"-test.run=^$",
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	gotPaths := []string{<-requestPaths, <-requestPaths}
	wantPaths := []string{"/" + checkID.String() + "/start", "/" + checkID.String() + "/0"}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("request paths = %v, want %v", gotPaths, wantPaths)
	}
}

func TestExecCommandRejectsEmptyCheckFlag(t *testing.T) {
	t.Parallel()

	factoryCalled := false
	cmd := rootCommandWithClientFactory(func(hc.RetryConfig) (*http.Client, error) {
		factoryCalled = true
		return http.DefaultClient, nil
	})
	cmd.SetArgs([]string{"exec", "--check=", "--", os.Args[0], "-test.run=^$"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want empty check flag error")
	}
	if factoryCalled {
		t.Fatal("client factory called for empty check flag")
	}
}

//nolint:paralleltest // helper subprocess mutates process-wide transport and exits intentionally.
func TestExecCommandHelper(t *testing.T) {
	if os.Getenv(execCommandHelperEnv) != "1" {
		return
	}

	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) < 2 {
		t.Fatalf("helper mode missing arguments: %v", os.Args)
	}

	switch args[1] {
	case "exec", "exec-env":
		if len(args) != 4 {
			t.Fatalf("exec helper got args %v, want %s <server-url> <check-id>", args[1:], args[1])
		}
		if args[1] == "exec-env" {
			t.Setenv(envCheckID, args[3])
		}
		runExecCommandHelper(t, args[2], args[3], args[1] == "exec-env")
	case "exit":
		if len(args) != 3 {
			t.Fatalf("exit helper got args %v, want exit <code>", args[1:])
		}
		code, err := strconv.Atoi(args[2])
		if err != nil {
			t.Fatalf("strconv.Atoi(%q) error = %v", args[2], err)
		}
		os.Exit(code)
	case "sleep-exit":
		if len(args) != 4 {
			t.Fatalf("sleep-exit helper got args %v, want sleep-exit <duration> <code>", args[1:])
		}
		delay, err := time.ParseDuration(args[2])
		if err != nil {
			t.Fatalf("time.ParseDuration(%q) error = %v", args[2], err)
		}
		code, err := strconv.Atoi(args[3])
		if err != nil {
			t.Fatalf("strconv.Atoi(%q) error = %v", args[3], err)
		}
		time.Sleep(delay)
		os.Exit(code)
	default:
		t.Fatalf("unknown helper mode %q", args[1])
	}
}

func runExecCommandHelper(t *testing.T, serverURL, checkID string, useEnvironment bool) {
	t.Helper()

	cmd := rootCommandWithClientFactory(routeHealthchecksTo(t, serverURL))
	args := []string{
		"exec",
		"--total-ping-timeout",
		"25ms",
	}
	if !useEnvironment {
		args = append(args, "--check", checkID)
	}
	args = append(args,
		"--",
		os.Args[0],
		"-test.run=TestExecCommandHelper",
		"--",
		"sleep-exit",
		"100ms",
		"7",
	)
	cmd.SetArgs(args)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("exec command error = %v", err)
	}
	fmt.Fprintln(os.Stderr, "exec command returned without exiting")
	os.Exit(0)
}
