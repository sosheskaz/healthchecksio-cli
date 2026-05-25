// Package cmd is the package that contains the CLI commands.
package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

type invalidCLIArgsError struct {
	Arg     string
	Problem string
}

func (e *invalidCLIArgsError) Error() string {
	return fmt.Sprintf("invalid argument: %s (%s)", e.Arg, e.Problem)
}

type healthChecksIOError struct {
	err        error
	StatusCode int
}

func (e *healthChecksIOError) Error() string {
	if e.err != nil {
		return fmt.Sprintf("healthchecks.io error: %+v", e.err)
	}
	return fmt.Sprintf("healthchecks.io error: status code %d", e.StatusCode)
}

var topCommands = make([]func() *cobra.Command, 0)

func rootCmdUsage() string {
	return fmt.Sprintf(`Usage: %s <check_id> [<signal>]
  <check_id> - The check id to be used
  <signal> - The signal to be sent, if any. Example: start, success, <return-code>, etc. See the docs for more details.
`, os.Args[0])
}

func mustWrite(w io.Writer, msg string) {
	if _, err := w.Write([]byte(msg)); err != nil {
		panic(err)
	}
}

func rootCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "healthchecksio-cli <check_id> [<signal>]",
		Short: "Call healthchecks.io checks from the command line",
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				checkID string
				signal  string
			)

			if len(args) == 0 {
				mustWrite(cmd.ErrOrStderr(), rootCmdUsage())
				return &invalidCLIArgsError{"posargs", "check_id not provided"}
			}
			if len(args) > 2 {
				mustWrite(cmd.ErrOrStderr(), rootCmdUsage())
				return &invalidCLIArgsError{"posargs", "too many arguments"}
			}
			checkID = args[0]
			if len(args) > 1 {
				signal = args[1]
			}

			mustWrite(cmd.ErrOrStderr(), "calling check "+checkID)
			if signal != "" {
				mustWrite(cmd.ErrOrStderr(), " with signal "+signal)
			}
			mustWrite(cmd.ErrOrStderr(), "\n")

			url := "https://hc-ping.com/" + checkID
			if signal != "" {
				url += "/" + signal
			}

			req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, url, http.NoBody)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to GET %q: %w", url, err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				return &healthChecksIOError{StatusCode: resp.StatusCode}
			}

			return nil
		},
	}

	for _, topCmd := range topCommands {
		c.AddCommand(topCmd())
	}

	c.InitDefaultCompletionCmd()
	c.Args = cobra.RangeArgs(1, 2)

	return c
}

// Command returns the root command for the healthchecksio-cli application.
func Command() *cobra.Command {
	return rootCommand()
}

// Execute runs the root command.
func Execute() {
	err := Command().Execute()
	if err != nil {
		os.Exit(1)
	}
}
