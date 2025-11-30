package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

const (
	rootCmdUsage = `Usage: %s <check_id> [<signal>]
  <check_id> - The check id to be used
  <signal> - The signal to be sent, if any. Example: start, success, <return-code>, etc. See the docs for more details.
`
)

var (
	topCommands = make([]func() *cobra.Command, 0)
)

func mustWrite(w io.Writer, msg string) {
	if _, err := w.Write([]byte(msg)); err != nil {
		panic(err)
	}
}

func setGlobalCommandFlags(cmd *cobra.Command) {
	input, err := cmd.Flags().GetString("input")
	if err == nil && input != "" {
		reader, err := os.Open(input)
		if err != nil {
			panic(fmt.Sprintf("failed to open input file %q: %+v", input, err))
		}
		cmd.SetIn(reader)
	}
	output, err := cmd.Flags().GetString("output")
	if err == nil && output != "" {
		writer, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			panic(fmt.Sprintf("failed to open output file %q: %+v", output, err))
		}
		cmd.SetOut(writer)
	}

	oldPostRun := cmd.PostRun
	cmd.PostRun = func(cmd *cobra.Command, args []string) {
		if oldPostRun != nil {
			oldPostRun(cmd, args)
		}
		if input != "" {
			_ = cmd.InOrStdin().(*os.File).Close()
		}
		if output != "" {
			_ = cmd.OutOrStdout().(*os.File).Close()
		}
	}
}

func rootCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "healthchecksio-cli <check_id> [<signal>]",
		Short: "Call healthchecks.io checks from the command line",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			setGlobalCommandFlags(cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				checkId string
				signal  string
			)

			if len(args) == 0 {
				mustWrite(cmd.ErrOrStderr(), rootCmdUsage)
				return errors.New("please provide a check id")
			}
			if len(args) > 2 {
				mustWrite(cmd.ErrOrStderr(), rootCmdUsage)
				return fmt.Errorf("extraneous arguments found: %v", args[2:])
			}
			checkId = args[0]
			if len(args) > 1 {
				signal = args[1]
			}

			mustWrite(cmd.ErrOrStderr(), "calling check "+checkId)
			if signal != "" {
				mustWrite(cmd.ErrOrStderr(), " with signal "+signal)
			}
			mustWrite(cmd.ErrOrStderr(), "\n")

			url := "https://hc-ping.com/" + checkId
			if signal != "" {
				url += "/" + signal
			}

			resp, err := http.Get(url)
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != 200 {
				return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			}

			return nil
		},
	}

	for _, topCmd := range topCommands {
		c.AddCommand(topCmd())
	}

	return c
}

func Command() *cobra.Command {
	return rootCommand()
}

func Execute() {
	err := Command().Execute()
	if err != nil {
		os.Exit(1)
	}
}
