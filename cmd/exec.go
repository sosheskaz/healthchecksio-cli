package cmd

import (
	"net/http"
	"os"
	"os/exec"
	"strconv"

	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec [flags] [command...]",
	Short: "Execute a command and report its status to healthchecks.io",
	Run: func(cmd *cobra.Command, args []string) {
		checkId := cmd.Flag("check").Value.String()
		if checkId == "" {
			cmd.Println("Please provide a check id")
			return
		}

		subcommand := exec.Command(args[0], args[1:]...)
		subcommand.Stdout = cmd.OutOrStdout()
		subcommand.Stderr = cmd.ErrOrStderr()

		mustWrite(cmd.ErrOrStderr(), "starting check "+checkId+"\n")
		if resp, err := http.Get("https://hc-ping.com/" + checkId + "/start"); err != nil {
			panic(err)
		} else {
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != 200 {
				panic(err)
			}
		}

		if err := subcommand.Start(); err != nil {
			panic(err)
		}

		if err := subcommand.Wait(); err != nil {
			if _, ok := err.(*exec.ExitError); !ok {
				panic(err)
			}
		}

		succeeded := subcommand.ProcessState.Success()

		var (
			resp  *http.Response
			hcErr error
		)

		if succeeded {
			//nolint:errcheck
			mustWrite(cmd.ErrOrStderr(), "check succeeded\n")
			resp, hcErr = http.Get("https://hc-ping.com/" + checkId)
		} else {
			//nolint:errcheck
			mustWrite(cmd.ErrOrStderr(), "check failed\n")
			resp, hcErr = http.Get("https://hc-ping.com/" + checkId + "/" + strconv.Itoa(subcommand.ProcessState.ExitCode()))
		}

		if hcErr != nil {
			panic(hcErr)
		} else if resp.StatusCode != 200 {
			panic(hcErr)
		}
		defer func() { _ = resp.Body.Close() }()

		os.Exit(subcommand.ProcessState.ExitCode())
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
	execCmd.Flags().StringP("check", "c", "", "The check id to be used")
	if err := execCmd.MarkFlagRequired("check"); err != nil {
		panic(err)
	}
	execCmd.Args = cobra.MinimumNArgs(1)
}
