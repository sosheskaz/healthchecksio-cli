package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"uuid"

	"github.com/spf13/cobra"
)

func execCommand(pingOpts *pingOptions, clientFactory pingClientFactory) *cobra.Command {
	c := &cobra.Command{
		Use:   "exec [flags] [command...]",
		Short: "Execute a command and report its status to healthchecks.io",
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			return bindAndValidatePingEnvironment(cmd, pingOpts)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			checkID := cmd.Flag("check").Value.String()
			if checkID == "" {
				return &invalidCLIArgsError{Arg: "check", Problem: "check id not provided"}
			}
			checkUUID, err := uuid.Parse(checkID)
			if err != nil {
				if environment := environmentSource(cmd, "check"); environment != "" {
					return invalidEnvironmentValueError(environment, "a valid UUID")
				}
				return fmt.Errorf("check ID is not a valid UUID: %w", err)
			}

			subcommand := exec.CommandContext(cmd.Context(), args[0], args[1:]...) //nolint:gosec // user-provided command
			subcommand.Stdin = cmd.InOrStdin()
			subcommand.Stdout = cmd.OutOrStdout()
			subcommand.Stderr = cmd.ErrOrStderr()

			check, err := pingOpts.newCheck(checkUUID, clientFactory)
			if err != nil {
				panic(fmt.Sprintf("failed to construct check: %+v", err))
			}

			mustWrite(cmd.ErrOrStderr(), "starting check\n")
			if err := pingOpts.call(cmd.Context(), func(ctx context.Context) error {
				return check.Start(ctx)
			}); err != nil {
				panic(fmt.Sprintf("failed to start check: %+v", err))
			}

			if err := subcommand.Start(); err != nil {
				panic(fmt.Sprintf("failed to start subcommand %q: %+v", strings.Join(subcommand.Args, " "), err))
			}

			if err := subcommand.Wait(); err != nil {
				exitError := &exec.ExitError{}
				if !errors.As(err, &exitError) {
					panic(err)
				}
			}

			exitCode := subcommand.ProcessState.ExitCode()
			mustWrite(cmd.ErrOrStderr(), fmt.Sprintf("completed with exit code %d\n", exitCode))

			if err := pingOpts.call(cmd.Context(), func(ctx context.Context) error {
				return check.CompleteStatus(ctx, exitCode)
			}); err != nil {
				panic(fmt.Sprintf("failed to complete check: %+v", err))
			}

			if exitCode != 0 {
				mustWrite(cmd.ErrOrStderr(), "check failed\n")
				os.Exit(exitCode)
			}
			mustWrite(cmd.ErrOrStderr(), "check succeeded\n")
			return nil
		},
	}

	c.Flags().StringP("check", "c", "", "The check id to be used (env: "+envCheckID+")")
	if err := c.MarkFlagRequired("check"); err != nil {
		panic(err)
	}
	c.Args = cobra.MinimumNArgs(1)

	return c
}

func init() {
	topCommands = append(topCommands, execCommand)
}
