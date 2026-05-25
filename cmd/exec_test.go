package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"github.com/google/uuid"
)

const execCommandHelperEnv = "HEALTHCHECKSIO_CLI_EXEC_HELPER"

func TestExecCommandReportsSubcommandExitCode(t *testing.T) {
	t.Parallel()

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
		context.Background(),
		os.Args[0],
		"-test.run=TestExecCommandHelper",
		"--",
		"exec",
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
	case "exec":
		if len(args) != 4 {
			t.Fatalf("exec helper got args %v, want exec <server-url> <check-id>", args[1:])
		}
		routeHealthchecksTo(t, args[2])
		runExecCommandHelper(t, args[3])
	case "exit":
		if len(args) != 3 {
			t.Fatalf("exit helper got args %v, want exit <code>", args[1:])
		}
		code, err := strconv.Atoi(args[2])
		if err != nil {
			t.Fatalf("strconv.Atoi(%q) error = %v", args[2], err)
		}
		os.Exit(code)
	default:
		t.Fatalf("unknown helper mode %q", args[1])
	}
}

func runExecCommandHelper(t *testing.T, checkID string) {
	t.Helper()

	cmd := execCommand()
	cmd.SetArgs([]string{
		"--check",
		checkID,
		"--",
		os.Args[0],
		"-test.run=TestExecCommandHelper",
		"--",
		"exit",
		"7",
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("exec command error = %v", err)
	}
	fmt.Fprintln(os.Stderr, "exec command returned without exiting")
	os.Exit(0)
}
