package cmd

import (
	"net/http"
	"os/exec"
	"strconv"

	"github.com/spf13/cobra"
)

// execCmd represents the exec command
var execCmd = &cobra.Command{
	Use:   "exec [flags] [command...]",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		checkId := cmd.Flag("check").Value.String()
		if checkId == "" {
			cmd.Println("Please provide a check id")
			return
		}

		subcommand := exec.Command(args[0], args[1:]...)
		subcommand.Stdout = cmd.OutOrStdout()
		subcommand.Stderr = cmd.OutOrStderr()

		cmd.OutOrStderr().Write([]byte("starting check " + checkId + "\n"))
		if resp, err := http.Get("https://hc-ping.com/" + checkId + "/start"); err != nil {
			cmd.OutOrStderr().Write([]byte(err.Error()))
			return
		} else if resp.StatusCode != 200 {
			cmd.OutOrStderr().Write([]byte("unexpected status code: " + strconv.Itoa(resp.StatusCode)))
			return
		}

		err := subcommand.Start()
		if err != nil {
			cmd.OutOrStderr().Write([]byte(err.Error()))
			return
		}

		if err := subcommand.Wait(); err != nil {
			_, ok := err.(*exec.ExitError)
			if !ok {
				cmd.OutOrStderr().Write([]byte(err.Error()))
				return
			}
		}

		succeeded := subcommand.ProcessState.Success()

		if succeeded {
			cmd.ErrOrStderr().Write([]byte("check succeeded\n"))
			http.Get("https://hc-ping.com/" + checkId)
		} else {
			cmd.ErrOrStderr().Write([]byte("check failed\n"))
			http.Get("https://hc-ping.com/" + checkId + "/" + strconv.Itoa(subcommand.ProcessState.ExitCode()))
		}
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
	execCmd.Flags().StringP("check", "c", "", "The check id to be used")
	execCmd.MarkFlagRequired("check")
	execCmd.Args = cobra.MinimumNArgs(1)
}
