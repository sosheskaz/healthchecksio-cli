// Package cmd is the package that contains the CLI commands.
package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/sosheskaz/healthchecksio-cli/internal/hc"
	"github.com/sosheskaz/healthchecksio-cli/internal/version"
)

type invalidCLIArgsError struct {
	Arg     string
	Problem string
}

func (e *invalidCLIArgsError) Error() string {
	return fmt.Sprintf("invalid argument: %s (%s)", e.Arg, e.Problem)
}

var topCommands = make([]func(*pingOptions, pingClientFactory) *cobra.Command, 0)

func rootCmdUsage() string {
	return fmt.Sprintf(`Usage: %s [<check_id>] [<signal>]
  <check_id> - The check id to be used (or set %s)
  <signal> - The signal to be sent, if any. Example: start, success, <return-code>, etc. See the docs for more details.
`, os.Args[0], envCheckID)
}

func mustWrite(w io.Writer, msg string) {
	if _, err := w.Write([]byte(msg)); err != nil {
		panic(err)
	}
}

func callbackForSignal(cmd *cobra.Command, check *hc.Check, signal string) (func(context.Context) error, error) {
	switch signal {
	case "":
		return func(ctx context.Context) error {
			return check.Success(ctx)
		}, nil
	case "failure", "fail", "false":
		return func(ctx context.Context) error {
			return check.Failure(ctx)
		}, nil
	case "success", "true":
		return func(ctx context.Context) error {
			return check.Success(ctx)
		}, nil
	case "log":
		message, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, fmt.Errorf("failed to read log message: %w", err)
		}
		return func(ctx context.Context) error {
			return check.Log(ctx, string(message))
		}, nil
	case "start":
		return func(ctx context.Context) error {
			return check.Start(ctx)
		}, nil
	default:
		statusCode, err := strconv.Atoi(signal)
		if err != nil {
			return nil, &invalidCLIArgsError{Arg: "signal", Problem: fmt.Sprintf("illegal value %q", signal)}
		}
		return func(ctx context.Context) error {
			return check.CompleteStatus(ctx, statusCode)
		}, nil
	}
}

func rootCommand() *cobra.Command {
	return rootCommandWithClientFactory(hc.NewRetryingHTTPClient)
}

func rootCommandWithClientFactory(clientFactory pingClientFactory) *cobra.Command {
	pingOpts := defaultPingOptions()
	c := &cobra.Command{
		Use:     "healthchecksio-cli [<check_id>] [<signal>]",
		Short:   "Call healthchecks.io checks from the command line",
		Version: version.Get().String(),
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			return bindAndValidatePingEnvironment(cmd, pingOpts)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			checkID, signal, checkIDEnvironment := directPingArguments(args)
			if checkID == "" {
				mustWrite(cmd.ErrOrStderr(), rootCmdUsage())
				return &invalidCLIArgsError{"posargs", "check_id not provided"}
			}
			checkUUID, err := uuid.Parse(checkID)
			if err != nil {
				if checkIDEnvironment != "" {
					return invalidEnvironmentValueError(checkIDEnvironment, "a valid UUID")
				}
				return fmt.Errorf("invalid check_id: %w", err)
			}

			check, err := pingOpts.newCheck(checkUUID, clientFactory)
			if err != nil {
				return fmt.Errorf("failed to create check: %w", err)
			}

			callback, err := callbackForSignal(cmd, check, signal)
			if err != nil {
				return err
			}

			if err := pingOpts.call(cmd.Context(), callback); err != nil {
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
		c.AddCommand(topCmd(pingOpts, clientFactory))
	}

	c.PersistentFlags().IntVar(
		&pingOpts.attempts,
		"attempts",
		defaultAttempts,
		"Total HTTP attempts per ping (0 to retry indefinitely within the total ping timeout; env: "+envAttempts+")",
	)
	c.PersistentFlags().DurationVar(
		&pingOpts.retryMaxBackoff,
		"retry-max-backoff",
		defaultRetryMaxBackoff,
		"Maximum delay between ping attempts (env: "+envRetryMaxBackoff+")",
	)
	c.PersistentFlags().DurationVar(
		&pingOpts.connectionTimeout,
		"connection-timeout",
		defaultConnectionTimeout,
		"Timeout for ping connection and TLS setup (env: "+envConnectionTimeout+")",
	)
	c.PersistentFlags().DurationVar(
		&pingOpts.totalPingTimeout,
		"total-ping-timeout",
		defaultTotalPingTimeout,
		"Total timeout for each ping operation (env: "+envTotalPingTimeout+")",
	)

	c.InitDefaultCompletionCmd()
	c.Args = cobra.MaximumNArgs(2)

	return c
}

func directPingArguments(args []string) (string, string, string) {
	switch len(args) {
	case 0:
		checkID := environmentCheckID()
		if checkID == "" {
			return "", "", ""
		}
		return checkID, "", envCheckID
	case 1:
		if args[0] == "" {
			return "", "", ""
		}
		if _, err := uuid.Parse(args[0]); err == nil {
			return args[0], "", ""
		}
		if checkID := environmentCheckID(); checkID != "" {
			return checkID, args[0], envCheckID
		}
		return args[0], "", ""
	default:
		return args[0], args[1], ""
	}
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
