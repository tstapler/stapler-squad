package config

// Log rotation defaults. At ~11 MB/hour the old 10 MB x 5 policy kept only ~4.5 hours of history.
const (
	DefaultLogMaxSizeMB  = 50
	DefaultLogMaxFiles   = 100
	DefaultLogMaxAgeDays = 14
)

// The old defaults were written verbatim into every existing config.json, so raising the
// constants alone would never reach installs that never customized them.
const (
	legacyLogMaxSizeMB  = 10
	legacyLogMaxFiles   = 5
	legacyLogMaxAgeDays = 30
)

// upgradeLegacyLogRotation replaces the exact old default triple with the current defaults;
// any other combination is a deliberate user choice and is left alone.
func upgradeLegacyLogRotation(cfg *Config) {
	if cfg.LogMaxSize == legacyLogMaxSizeMB && cfg.LogMaxFiles == legacyLogMaxFiles && cfg.LogMaxAge == legacyLogMaxAgeDays {
		cfg.LogMaxSize = DefaultLogMaxSizeMB
		cfg.LogMaxFiles = DefaultLogMaxFiles
		cfg.LogMaxAge = DefaultLogMaxAgeDays
	}
}
