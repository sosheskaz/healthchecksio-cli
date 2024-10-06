package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
)

func main() {
	args, err := GetArgs()
	if err != nil {
		fmt.Println(Usage())
		fmt.Println(err)
		os.Exit(1)
	}

	var actionMessage string
	url := "https://hc-ping.com/" + args.CheckId
	if args.Signal != "" {
		url += "/" + args.Signal
		actionMessage = fmt.Sprintf("Calling %s on check %s", args.Signal, args.CheckId)
	} else {
		actionMessage = fmt.Sprintf("Calling check %s", args.CheckId)
	}

	fmt.Println(actionMessage)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	fmt.Println("Response:", resp.Status)
	if resp.StatusCode != 200 {
		os.Exit(1)
	}
}

type Args struct {
	CheckId        string
	Signal         string
	extraneousArgs []string
}

func GetArgs() (Args, error) {
	var err error

	args := os.Args[1:]

	if len(args) == 0 {
		fmt.Println("Please provide a check id")
		return Args{}, errors.New("no check id provided")
	}

	result := Args{
		CheckId: args[0],
	}
	if len(args) > 1 {
		result.Signal = args[1]
	}
	if len(args) > 2 {
		result.extraneousArgs = args[2:]
		err = errors.New("extraneous arguments found")
	}

	return result, err
}

func Usage() string {
	executable := os.Args[0]

	var (
		checkId        string = "not-set"
		signal         string = "not-set"
		extraneousArgs []string
	)
	if len(os.Args) > 1 {
		checkId = os.Args[1]
	}
	if len(os.Args) > 2 {
		signal = os.Args[2]
		extraneousArgs = os.Args[3:]
	}

	return fmt.Sprintf(`Usage: %s <check_id> [<signal>]
  <check_id> - The check id to be used
  <signal> - The signal to be sent, if any. Example: start, success, <return-code>, etc. See the docs for more details.

  Found args:
    check_id: %s
    signal: %s
    extraneousArgs: %+v
`, executable, checkId, signal, extraneousArgs)
}
