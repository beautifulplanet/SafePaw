package secrets

import (
	"context"
	"os"
	"testing"
)

func TestEnvProvider_Get(t *testing.T) {
	p := EnvProvider{}

	t.Run("existing variable", func(t *testing.T) {
		const key = "TEST_SECRETS_PROVIDER_EXISTING"
		os.Setenv(key, "hunter2")
		t.Cleanup(func() { os.Unsetenv(key) })

		val, err := p.Get(context.Background(), key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != "hunter2" {
			t.Fatalf("got %q, want %q", val, "hunter2")
		}
	})

	t.Run("missing variable returns empty", func(t *testing.T) {
		val, err := p.Get(context.Background(), "TEST_SECRETS_PROVIDER_NONEXISTENT_KEY_XYZ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != "" {
			t.Fatalf("got %q, want empty string", val)
		}
	})
}
