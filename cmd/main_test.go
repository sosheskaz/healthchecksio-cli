package cmd

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	for _, binding := range envFlagBindings {
		if err := os.Unsetenv(binding.environment); err != nil {
			panic(fmt.Sprintf("unset %s: %v", binding.environment, err))
		}
	}

	os.Exit(m.Run())
}
