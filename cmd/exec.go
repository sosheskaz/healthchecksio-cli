package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"

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

			subcommand := exec.CommandContext(cmd.Context(), args[0], args[1:]...) //nolint:gosec // user-provided command
			subcommand.Stdout = cmd.OutOrStdout()
			subcommand.Stderr = cmd.ErrOrStderr()

			mustWrite(cmd.ErrOrStderr(), "starting check "+checkID+"\n")
			startReq, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, "https://hc-ping.com/"+checkID+"/start", http.NoBody)
			if err != nil {
				panic(err)
			}
			startResp, err := http.DefaultClient.Do(startReq)
			if err != nil {
				panic(err)
			}
			defer func() { _ = startResp.Body.Close() }()

			if startResp.StatusCode != http.StatusOK {
				bodyData, err := io.ReadAll(startResp.Body)
				if err != nil {
					panic(fmt.Sprintf("failed to read response body: %+v", err))
				}
				req := startResp.Request
				panic(fmt.Sprintf("received unexpected status code from %s %s: %d\n%s", req.Method, req.URL, startResp.StatusCode, string(bodyData)))
			}

			if err := subcommand.Start(); err != nil {
				panic(err)
			}

			if err := subcommand.Wait(); err != nil {
				exitError := &exec.ExitError{}
				if errors.As(err, &exitError) {
					panic(err)
				}
			}

			succeeded := subcommand.ProcessState.Success()

			var (
				endResp *http.Response
				hcErr   error
			)

			if succeeded {
				mustWrite(cmd.ErrOrStderr(), "check succeeded\n")
				url := "https://hc-ping.com/" + checkID
				req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, url, http.NoBody)
				if err != nil {
					panic(err)
				}
				endResp, hcErr = http.DefaultClient.Do(req)
			} else {
				mustWrite(cmd.ErrOrStderr(), "check failed\n")
				url := "https://hc-ping.com/" + checkID + "/" + strconv.Itoa(subcommand.ProcessState.ExitCode())
				req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, url, http.NoBody)
				if err != nil {
					panic(err)
				}
				endResp, hcErr = http.DefaultClient.Do(req)
			}

			if hcErr != nil {
				panic(hcErr)
			} else if endResp.StatusCode != http.StatusOK {
				bodyData, err := io.ReadAll(endResp.Body)
				if err != nil {
					panic(fmt.Sprintf("failed to read response body: %+v", err))
				}
				panic(fmt.Sprintf("received unexpected status code from %s %s: %d\n%s", endResp.Request.Method, endResp.Request.URL, endResp.StatusCode, string(bodyData)))
			}
			defer func() { _ = endResp.Body.Close() }()

			os.Exit(subcommand.ProcessState.ExitCode())
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
