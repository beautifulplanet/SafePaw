// Package secrets defines the SecretsProvider interface for pluggable secret
// backends. The default implementation reads from environment variables (the
// current SafePaw behavior). Alternative implementations can read from
// HashiCorp Vault, AWS Secrets Manager, GCP Secret Manager, etc.
//
// This interface exists as a documented migration path — SafePaw does not
// currently require an external secrets manager. See docs/SECRETS-MIGRATION.md.
package secrets

import "context"

// Provider retrieves secret values by key. Implementations must be safe
// for concurrent use.
type Provider interface {
	// Get returns the secret value for the given key.
	// Returns ("", nil) if the key is not found (missing secret ≠ error).
	// Returns ("", err) on backend communication failure.
	Get(ctx context.Context, key string) (string, error)
}

// Well-known secret key constants used across SafePaw services.
const (
	KeyAuthSecret         = "AUTH_SECRET"
	KeyRedisPassword      = "REDIS_PASSWORD"
	KeyPostgresPassword   = "POSTGRES_PASSWORD"
	KeyWizardAdminPass    = "WIZARD_ADMIN_PASSWORD"
	KeyWizardOperatorPass = "WIZARD_OPERATOR_PASSWORD"
	KeyWizardViewerPass   = "WIZARD_VIEWER_PASSWORD"
	KeyWizardTOTPSecret   = "WIZARD_TOTP_SECRET"
	KeyAnthropicAPIKey    = "ANTHROPIC_API_KEY"
	KeyOpenAIAPIKey       = "OPENAI_API_KEY"
	KeyOpenClawToken      = "OPENCLAW_GATEWAY_TOKEN"
)
