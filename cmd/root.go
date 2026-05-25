// Package cmd is the package that contains the CLI commands.
package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/google/uuid"
	"github.com/sosheskaz/healthchecksio-cli/internal/hc"
	"github.com/spf13/cobra"
)

type invalidCLIArgsError struct {
	Arg     string
	Problem string
}

func (e *invalidCLIArgsError) Error() string {
	return fmt.Sprintf("invalid argument: %s (%s)", e.Arg, e.Problem)
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
			checkUUID, err := uuid.Parse(checkID)
			if err != nil {
				return fmt.Errorf("invalid check_id: %w", err)
			}

			check, err := hc.NewUUIDCheck(checkUUID)
			if err != nil {
				return fmt.Errorf("failed to create check: %w", err)
			}

			var callback func(context.Context) error
			switch signal {
			case "":
				callback = func(ctx context.Context) error {
					return check.Success(ctx)
				}
			case "failure", "fail", "false":
				callback = func(ctx context.Context) error {
					return check.Failure(ctx)
				}
			case "success", "true":
				callback = func(ctx context.Context) error {
					return check.Success(ctx)
				}
			case "log":
				callback = func(ctx context.Context) error {
					message, err := io.ReadAll(cmd.InOrStdin())
					if err != nil {
						return fmt.Errorf("failed to read log message: %w", err)
					}
					return check.Log(ctx, string(message))
				}
			default:
				statusCode, err := strconv.Atoi(signal)
				if err != nil {
					return &invalidCLIArgsError{Arg: "signal", Problem: fmt.Sprintf("illegal value %q", signal)}
				}
				callback = func(ctx context.Context) error {
					return check.CompleteStatus(ctx, statusCode)
				}
			}

			if err := callback(cmd.Context()); err != nil {
				return err
			}

			mustWrite(cmd.ErrOrStderr(), "calling check "+checkID)
			if signal != "" {
				mustWrite(cmd.ErrOrStderr(), " with signal "+signal)
			}
			mustWrite(cmd.ErrOrStderr(), "\n")

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
