package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var errInvalidEnvironmentValue = errors.New("invalid environment variable value")

const (
	envCheckID           = "HEALTHCHECKSIO_CHECK_ID"
	envAttempts          = "HEALTHCHECKSIO_ATTEMPTS"
	envRetryMaxBackoff   = "HEALTHCHECKSIO_RETRY_MAX_BACKOFF"
	envConnectionTimeout = "HEALTHCHECKSIO_CONNECTION_TIMEOUT"
	envTotalPingTimeout  = "HEALTHCHECKSIO_TOTAL_PING_TIMEOUT"
)

type envFlagBinding struct {
	environment string
	flag        string
}

type environmentSourcesKey struct{}

var envFlagBindings = []envFlagBinding{
	{environment: envCheckID, flag: "check"},
	{environment: envAttempts, flag: "attempts"},
	{environment: envRetryMaxBackoff, flag: "retry-max-backoff"},
	{environment: envConnectionTimeout, flag: "connection-timeout"},
	{environment: envTotalPingTimeout, flag: "total-ping-timeout"},
}

func bindEnvironment(cmd *cobra.Command) error {
	flags := cmd.Flags()
	sources := make(map[string]string)
	for _, binding := range envFlagBindings {
		flag := flags.Lookup(binding.flag)
		if flag == nil || flag.Changed {
			continue
		}

		value, ok := os.LookupEnv(binding.environment)
		if !ok || value == "" {
			continue
		}
		if err := flags.Set(binding.flag, value); err != nil {
			return invalidEnvironmentValueError(binding.environment, flag.Value.Type())
		}
		sources[binding.flag] = binding.environment
	}
	cmd.SetContext(context.WithValue(cmd.Context(), environmentSourcesKey{}, sources))

	return nil
}

func bindAndValidatePingEnvironment(cmd *cobra.Command, pingOpts *pingOptions) error {
	if err := bindEnvironment(cmd); err != nil {
		return err
	}
	return environmentPingValidationError(cmd, pingOpts.validate())
}

func environmentSource(cmd *cobra.Command, flag string) string {
	sources, ok := cmd.Context().Value(environmentSourcesKey{}).(map[string]string)
	if !ok {
		return ""
	}
	return sources[flag]
}

func invalidEnvironmentValueError(environment, expected string) error {
	return fmt.Errorf("%w: %s expects %s", errInvalidEnvironmentValue, environment, expected)
}

func environmentPingValidationError(cmd *cobra.Command, validationErr error) error {
	var flag string
	switch {
	case errors.Is(validationErr, errNegativeAttempts):
		flag = "attempts"
	case errors.Is(validationErr, errNonPositiveBackoff):
		flag = "retry-max-backoff"
	case errors.Is(validationErr, errNonPositiveConnection):
		flag = "connection-timeout"
	case errors.Is(validationErr, errNonPositivePingTimeout):
		flag = "total-ping-timeout"
	default:
		return validationErr
	}

	environment := environmentSource(cmd, flag)
	if environment == "" {
		return validationErr
	}
	return fmt.Errorf("%w: %s: %w", errInvalidEnvironmentValue, environment, validationErr)
}

func environmentCheckID() string {
	return os.Getenv(envCheckID)
}
