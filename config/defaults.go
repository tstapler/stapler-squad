package config

import (
	"path/filepath"
	"strings"

	"github.com/tstapler/stapler-squad/log"
)

// ResolvedDefaults is the merged result of all applicable default layers for a new session.
type ResolvedDefaults struct {
	Program  string
	AutoYes  bool
	Tags     []string
	EnvVars  map[string]string
	CLIFlags string

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
