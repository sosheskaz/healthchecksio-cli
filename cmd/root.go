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

func mustWrite(w io.Writer, msg string) {
	if _, err := w.Write([]byte(msg)); err != nil {
		panic(err)
	}
}

var rootCmd = &cobra.Command{
	Use:   "healthchecksio-cli <check_id> [<signal>]",
	Short: "Call healthchecks.io checks from the command line",
	RunE: func(cmd *cobra.Command, args []string) error {
		var (
			checkId string
			signal  string
		)

		if len(args) == 0 {
			mustWrite(cmd.ErrOrStderr(), rootCmdUsage)
			return errors.New("Please provide a check id")
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
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		return nil
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.InitDefaultCompletionCmd()
	rootCmd.Args = cobra.RangeArgs(1, 2)
}
