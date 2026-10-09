package gitwiring

import (
	"os"
	"sort"
	"strings"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git/backend"
)

// EnvGitBackend set to "cli" forces every cohort to the CLI (emergency rollback, ADR-004).
const EnvGitBackend = "STAPLER_SQUAD_GIT_BACKEND"

// ParseCohortMap is the single parse-at-boundary owner of config.GitBackendCohorts.
// It is total: every invalid key or value logs a WARN naming the key and resolves to the
// CLI, and every unspecified cohort is the CLI. STAPLER_SQUAD_GIT_BACKEND=cli overrides all.
func ParseCohortMap(raw map[string]string) backend.CohortMap {
	return parseCohortMap(raw, os.Getenv(EnvGitBackend))
}

func parseCohortMap(raw map[string]string, envOverride string) backend.CohortMap {
	var cohorts backend.CohortMap
	switch envOverride {
	case "":
	case "cli":
		return cohorts
	default:
		log.Warn("ignoring invalid git backend env override (only \"cli\" is accepted)",
			"env", EnvGitBackend, "value", envOverride)
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
		if mode == backend.BackendShadow && !cohort.HasReadOperations() {
			log.Warn("shadow mode is not allowed for a write-only cohort, using cli", "key", key, "value", value)
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
