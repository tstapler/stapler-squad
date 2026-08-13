package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tstapler/stapler-squad/log"
)

// ErrAliasNotFound is returned by ResolveAlias when no alias matches the given name.
var ErrAliasNotFound = errors.New("alias not found")

// ResolvedDefaults is the merged result of all applicable default layers for a new session.
type ResolvedDefaults struct {
	Program  string
	AutoYes  bool
	Tags     []string
	EnvVars  map[string]string
	CLIFlags string

	// Path is the working directory path from an alias (empty for non-alias resolution).
	Path string
	// Branch is the git branch hint from alias invocation (e.g. from @alias:branch).
	Branch string
	// SessionLabel is the session label from alias invocation (text between alias and --)
	SessionLabel string

	// Source tracking — which layers contributed to this result.
	UsedGlobal       bool
	UsedDirectory    bool
	UsedProfile      bool
	MatchedDirectory string
}

// ResolveDefaults merges the three layers of session defaults (global → directory → profile)
// for the given working directory and optional profile name.
//
// Precedence (lowest → highest):
//  1. cfg.DefaultProgram (legacy fallback)
//  2. cfg.SessionDefaults global fields
//  3. DirectoryRule.Overrides for the longest-matching path prefix
//  4. Named profile (profileName argument)
//
// Merge semantics:
//   - Scalar fields (Program, CLIFlags): non-empty source value overwrites target
//   - AutoYes: true in any layer sets it true
//   - Tags: union across all layers (duplicates removed)
//   - EnvVars: higher-layer key overwrites lower-layer key
func ResolveDefaults(cfg *Config, workingDir, profileName string) ResolvedDefaults {
	result := ResolvedDefaults{
		EnvVars: make(map[string]string),
		Tags:    []string{},
	}

	// Layer 1: legacy DefaultProgram fallback
	if cfg.DefaultProgram != "" {
		result.Program = cfg.DefaultProgram
	}

	// Layer 2: global SessionDefaults
	sd := cfg.SessionDefaults
	if sd.Program != "" || sd.AutoYes || len(sd.Tags) > 0 || len(sd.EnvVars) > 0 || sd.CLIFlags != "" {
		result.UsedGlobal = true
	}
	mergeProfileInto(&result, ProfileDefaults{
		Program:  sd.Program,
		AutoYes:  sd.AutoYes,
		Tags:     sd.Tags,
		EnvVars:  sd.EnvVars,
		CLIFlags: sd.CLIFlags,
	})

	// Layer 3: directory rule (longest-prefix match)
	if workingDir != "" {
		rule := findClosestDirectoryRule(sd.DirectoryRules, workingDir)
		if rule != nil {
			result.UsedDirectory = true
			result.MatchedDirectory = rule.Path

			// If the rule references a named profile, apply that profile first.
			if rule.Profile != "" {
				if p, ok := sd.Profiles[rule.Profile]; ok {
					mergeProfileInto(&result, p)
				}
			}
			// Then apply the rule's own overrides (higher priority than the referenced profile).
			mergeProfileInto(&result, rule.Overrides)
		}
	}

	// Layer 4: explicitly requested profile (user selection wins last)
	if profileName != "" {
		if p, ok := sd.Profiles[profileName]; ok {
			result.UsedProfile = true
			mergeProfileInto(&result, p)
		}
	}

	return result
}

// findClosestDirectoryRule returns the DirectoryRule whose Path is the longest
// prefix of workingDir, or nil if no rule matches.
// Both paths are symlink-resolved before comparison.
func findClosestDirectoryRule(rules []DirectoryRule, workingDir string) *DirectoryRule {
	resolved := evalSymlinksOrOriginal(workingDir)
	var best *DirectoryRule
	for i := range rules {
		r := &rules[i]
		rPath := evalSymlinksOrOriginal(r.Path)
		if resolved == rPath || strings.HasPrefix(resolved, rPath+string(filepath.Separator)) {
			if best == nil || len(rPath) > len(evalSymlinksOrOriginal(best.Path)) {
				best = r
			}
		}
	}
	return best
}

// evalSymlinksOrOriginal resolves symlinks; returns the original path on error.
func evalSymlinksOrOriginal(path string) string {
	if path == "" {
		return path
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		log.Warn("could not resolve symlink", "path", path, "err", err)
		return path
	}
	return resolved
}

// mergeProfileInto applies src fields onto result.
// Non-zero scalar src fields overwrite result fields.
// Tags are unioned. EnvVars are merged (src key wins).
func mergeProfileInto(result *ResolvedDefaults, src ProfileDefaults) {
	if src.Program != "" {
		result.Program = src.Program
	}
	if src.AutoYes {
		result.AutoYes = true
	}
	if src.CLIFlags != "" {
		result.CLIFlags = src.CLIFlags
	}
	// Tags: union
	if len(src.Tags) > 0 {
		result.Tags = unionTags(result.Tags, src.Tags)
	}
	// EnvVars: merge (src key wins)
	for k, v := range src.EnvVars {
		result.EnvVars[k] = v
	}
}

// unionTags returns the union of two tag slices with duplicates removed.
func unionTags(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	result := make([]string, 0, len(a)+len(b))
	for _, t := range a {
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			result = append(result, t)
		}
	}
	for _, t := range b {
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			result = append(result, t)
		}
	}
	return result
}

// FindAlias returns the AliasConfig with the given name (case-insensitive), or nil if not found.
func FindAlias(cfg *Config, name string) *AliasConfig {
	for i := range cfg.SessionDefaults.Aliases {
		if strings.EqualFold(cfg.SessionDefaults.Aliases[i].Name, name) {
			return &cfg.SessionDefaults.Aliases[i]
		}
	}
	return nil
}

// GetAliasesByGroup groups all aliases by their Group field.
// Aliases without a Group are stored under the empty-string key "".
func GetAliasesByGroup(cfg *Config) map[string][]AliasConfig {
	result := make(map[string][]AliasConfig)
	for _, a := range cfg.SessionDefaults.Aliases {
		result[a.Group] = append(result[a.Group], a)
	}
	return result
}

var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// ExpandEnvVars expands ${VAR_NAME} tokens in map values using os.LookupEnv.
// If any referenced env var is not set (vs. set to ""), the key is omitted
// from the result and a warning is logged.
func ExpandEnvVars(m map[string]string) map[string]string {
	if len(m) == 0 {
		return m
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		allSet := true
		expanded := envVarPattern.ReplaceAllStringFunc(v, func(match string) string {
			varName := match[2 : len(match)-1] // strip ${ and }
			val, ok := os.LookupEnv(varName)
			if !ok {
				allSet = false
			}
			return val
		})
		if !allSet {
			log.Warn("alias env var not set, omitting key", "key", k)
			continue
		}
		result[k] = expanded
	}
	return result
}

// ResolveAlias resolves an alias by name and returns merged session defaults.
// Resolution order: global → directory → profile → alias inline fields.
// The alias's CLIFlags replace each prior layer (same as mergeProfileInto semantics).
// extraFlags are appended to the final CLIFlags as an explicit invocation-time step.
//
// Path, Branch, and SessionLabel are promoted into the returned ResolvedDefaults
// so callers do not need to access the raw AliasConfig.
func ResolveAlias(cfg *Config, aliasName, branch, label, extraFlags string) (ResolvedDefaults, error) {
	alias := FindAlias(cfg, aliasName)
	if alias == nil {
		return ResolvedDefaults{}, fmt.Errorf("alias %q: %w", aliasName, ErrAliasNotFound)
	}

	// Resolve base defaults (global → dir → profile).
	base := ResolveDefaults(cfg, alias.Path, alias.Profile)

	// Merge alias inline fields on top (same replace semantics as mergeProfileInto).
	mergeProfileInto(&base, ProfileDefaults{
		Program:  alias.Program,
		AutoYes:  alias.AutoYes,
		Tags:     alias.Tags,
		EnvVars:  alias.EnvVars,
		CLIFlags: alias.CLIFlags,
	})

	// Expand ${VAR} in env vars after all merge layers are applied.
	base.EnvVars = ExpandEnvVars(base.EnvVars)

	// Append invocation-time extraFlags (these are caller-supplied, not a config layer).
	// Resolution layers use REPLACE semantics; invocation-time flags use APPEND semantics.
	if extraFlags != "" {
		if base.CLIFlags != "" {
			base.CLIFlags += " " + extraFlags
		} else {
			base.CLIFlags = extraFlags
		}
	}

	// Promote alias path, branch, and label into the result.
	base.Path = alias.Path
	base.Branch = branch
	base.SessionLabel = label

	return base, nil
}
