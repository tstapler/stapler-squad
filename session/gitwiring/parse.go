package gitwiring

import (
	"os"
	"sort"
	"strings"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git/backend"
)

// EnvGitBackend forces every cohort to the CLI when set to any non-empty value (emergency rollback, ADR-004; it can only disable gogit).
const EnvGitBackend = "STAPLER_SQUAD_GIT_BACKEND"

// ParseCohortMap is the single parse-at-boundary owner of config.GitBackendCohorts.
// It is total: every invalid key or value logs a WARN naming the key and resolves to the
// CLI, and every unspecified cohort is the CLI. A non-empty STAPLER_SQUAD_GIT_BACKEND overrides all.
func ParseCohortMap(raw map[string]string) backend.CohortMap {
	return parseCohortMap(raw, os.Getenv(EnvGitBackend))
}

func parseCohortMap(raw map[string]string, envOverride string) backend.CohortMap {
	var cohorts backend.CohortMap
	if v := strings.TrimSpace(envOverride); v != "" {
		// Fail closed: the env var can only disable gogit, so any non-empty value forces the CLI.
		if !strings.EqualFold(v, "cli") {
			log.Warn("unrecognised git backend env override, forcing every cohort to cli (only \"cli\" is documented)",
				"env", EnvGitBackend, "value", envOverride)
		}
		return cohorts
	}

	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys) // deterministic WARN order

	for _, key := range keys {
		value := raw[key]
		cohort, ok := backend.ParseCohort(key)
		if !ok {
			warnUnknownKey(key)
			continue
		}
		mode, ok := backend.ParseBackendMode(value)
		if !ok {
			log.Warn("invalid git backend mode, using cli", "key", key, "value", value)
			continue
		}
		if err := cohort.ValidateMode(mode); err != nil {
			log.Warn("illegal git backend mode, using cli", "key", key, "value", value, "reason", err)
			continue
		}
		cohorts = cohorts.With(cohort, mode)
	}
	return cohorts
}

func warnUnknownKey(key string) {
	if strings.Contains(key, "/") {
		// Per-operation overrides need the closed OperationName set (Story 1.1.2).
		log.Warn("per-operation git backend override is not supported yet, ignoring", "key", key)
		return
	}
	log.Warn("unknown git backend cohort, ignoring", "key", key)
}
