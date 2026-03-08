package secrets

import (
	"context"
	"os"
)

// EnvProvider reads secrets from environment variables. This is the default
// provider and matches SafePaw's current behavior.
type EnvProvider struct{}

// Get returns the value of the environment variable named by key.
// Returns ("", nil) if the variable is not set.
func (EnvProvider) Get(_ context.Context, key string) (string, error) {
	return os.Getenv(key), nil
}
