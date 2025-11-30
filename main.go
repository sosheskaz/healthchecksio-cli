//go:generate go run gen.go

package main

import "github.com/sosheskaz/healthchecksio-cli/cmd"

func main() {
	cmd.Execute()
}
