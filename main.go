// Package main is the entrypoint for the healthchecks.io CLI

//go:generate go run gen.go
package main

import "github.com/sosheskaz/healthchecksio-cli/cmd"

func main() {
	cmd.Execute()
}
