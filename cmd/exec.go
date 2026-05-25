package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/sosheskaz/healthchecksio-cli/internal/hc"
	"github.com/spf13/cobra"
)

func execCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "exec [flags] [command...]",
		Short: "Execute a command and report its status to healthchecks.io",
		Run: func(cmd *cobra.Command, args []string) {
			checkID := cmd.Flag("check").Value.String()
			if checkID == "" {
				cmd.Println("Please provide a check id")
				return
			}
			checkUUID, err := uuid.Parse(checkID)
			if err != nil {
				panic(fmt.Sprintf("check ID %q is not a valid UUID: %+v", checkID, err))
			}

			subcommand := exec.CommandContext(cmd.Context(), args[0], args[1:]...) //nolint:gosec // user-provided command
			subcommand.Stdout = cmd.OutOrStdout()
			subcommand.Stderr = cmd.ErrOrStderr()

			check, err := hc.NewUUIDCheck(checkUUID)
			if err != nil {
				panic(fmt.Sprintf("failed to construct check: %+v", err))
			}

			mustWrite(cmd.ErrOrStderr(), "starting check "+checkID+"\n")
			if err := check.Start(cmd.Context()); err != nil {
				panic(fmt.Sprintf("failed to start check: %+v", err))
			}

			if err := subcommand.Start(); err != nil {
				panic(fmt.Sprintf("failed to start subcommand %q: %+v", strings.Join(slices.Concat([]string{subcommand.Path}, subcommand.Args), " "), err))
			}

			if err := subcommand.Wait(); err != nil {
				exitError := &exec.ExitError{}
				if errors.As(err, &exitError) {
					panic(err)
				}
			}

			exitCode := subcommand.ProcessState.ExitCode()
			mustWrite(cmd.ErrOrStderr(), fmt.Sprintf("completed with exit code %d", err))

			if err := check.CompleteStatus(cmd.Context(), exitCode); err != nil {
				panic(fmt.Sprintf("failed to complete check: %+v", err))
			}
			mustWrite(cmd.ErrOrStderr(), "check succeeded\n")

			if exitCode != 0 {
				os.Exit(exitCode)
			}
		},
	}

	c.Flags().StringP("check", "c", "", "The check id to be used")
	if err := c.MarkFlagRequired("check"); err != nil {
		panic(err)
	}
	c.Args = cobra.MinimumNArgs(1)

	return c
}

func init() {
	topCommands = append(topCommands, execCommand)
}
