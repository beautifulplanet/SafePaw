// =============================================================
// SafePaw Gateway — LLM Pricing Reference
// =============================================================
// Reference pricing per million tokens for common LLM providers.
// Used by the cost dashboard to show projected costs and for
// alert threshold context. Actual costs come from OpenClaw's
// usage.cost API — this is supplementary reference data only.
//
// Prices last updated: 2026-03-08
// Sources: Anthropic/OpenAI public pricing pages
// =============================================================

package main

// ModelPricing holds per-million-token pricing for a model.
type ModelPricing struct {
	Provider      string  `json:"provider"`
	Model         string  `json:"model"`
	InputPerM     float64 `json:"inputPerMillion"`     // USD per 1M input tokens
	OutputPerM    float64 `json:"outputPerMillion"`    // USD per 1M output tokens
	CacheReadPerM float64 `json:"cacheReadPerMillion"` // USD per 1M cache-read tokens (0 = N/A)
}

// PricingTable is the current reference pricing for cost estimation.
var PricingTable = []ModelPricing{
	// Anthropic
	{Provider: "anthropic", Model: "claude-sonnet-4-20250514", InputPerM: 3.00, OutputPerM: 15.00, CacheReadPerM: 0.30},
	{Provider: "anthropic", Model: "claude-opus-4-20250514", InputPerM: 15.00, OutputPerM: 75.00, CacheReadPerM: 1.50},
	{Provider: "anthropic", Model: "claude-3.5-haiku-20241022", InputPerM: 0.80, OutputPerM: 4.00, CacheReadPerM: 0.08},

	// OpenAI
	{Provider: "openai", Model: "gpt-4o", InputPerM: 2.50, OutputPerM: 10.00},
	{Provider: "openai", Model: "gpt-4o-mini", InputPerM: 0.15, OutputPerM: 0.60},
	{Provider: "openai", Model: "o3", InputPerM: 10.00, OutputPerM: 40.00},
	{Provider: "openai", Model: "o4-mini", InputPerM: 1.10, OutputPerM: 4.40},

	// Google
	{Provider: "google", Model: "gemini-2.5-pro", InputPerM: 1.25, OutputPerM: 10.00},
	{Provider: "google", Model: "gemini-2.5-flash", InputPerM: 0.15, OutputPerM: 0.60},
}
