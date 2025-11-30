//go:build ignore

package main

import (
	"fmt"
	"os"
	"path"

	"github.com/sosheskaz/healthchecksio-cli/cmd"
)

func main() {
	outDir := path.Join("completions")

	if err := os.RemoveAll(outDir); err != nil {
		panic(fmt.Sprintf("failed to remove output directory %q: %+v", outDir, err))
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		panic(fmt.Sprintf("failed to create output directory %q: %+v", outDir, err))
	}

	cmd := cmd.Command()
	errs := []error{
		cmd.GenBashCompletionFileV2(path.Join(outDir, "bash_completion.sh"), true),
		cmd.GenZshCompletionFile(path.Join(outDir, "zsh_completion.sh")),
		cmd.GenFishCompletionFile(path.Join(outDir, "fish_completion.fish"), true),
		cmd.GenPowerShellCompletionFileWithDesc(path.Join(outDir, "powershell_completion.ps1")),
	}
	for _, err := range errs {
		if err != nil {
			panic(fmt.Sprintf("failed to generate completion files: %+v", err))
		}
	}
}
