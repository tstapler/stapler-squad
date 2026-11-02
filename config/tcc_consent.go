package config

import "runtime"

const TCCEnrichmentConsentFlag = "tcc:enrichment-pre-consent"

// TCCEnrichmentConsentEnabled is a Darwin-only, default-on enrichment gate.
func (c *Config) TCCEnrichmentConsentEnabled() bool {
	return runtime.GOOS == "darwin" && c.GetFeatureFlagWithDefault(TCCEnrichmentConsentFlag, true)
}
