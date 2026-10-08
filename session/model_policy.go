package session

import (
	"strings"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

// ResolveFeatureModel returns the concrete model ID for a background feature, or "" for the
// account default. A non-claude program never receives a Claude model ID. An unresolvable
// configured value falls back to the built-in default (with a warning) rather than to the
// account default, so a typo cannot silently re-enable Opus-priced work.
func ResolveFeatureModel(cfg *config.Config, families map[string]string, key, program string) string {
	if program != "" && !isClaudeProgramString(program) {
		return ""
	}
	configured := cfg.ModelPolicyValue(key)
	if configured == "" {
		return ""
	}
	resolved, err := ResolveModel(families, configured)
	if err == nil {
		return resolved
	}
	log.Warn("model_policy value unresolvable, using built-in default", "key", key, "value", configured, "err", err)
	resolved, err = ResolveModel(families, config.ModelPolicyDefault(key))
	if err != nil {
		return ""
	}
	return resolved
}

// ApplyModelPolicyToProgram adds --model/--effort to a claude launch command from the
// policy for key. It returns "" (leave the program unchanged) when base is not a claude
// command, or already pins a model/effort itself.
func ApplyModelPolicyToProgram(cfg *config.Config, families map[string]string, key, base string) string {
	if !isClaudeProgramString(base) {
		return ""
	}
	out := base
	if !strings.Contains(base, "--model") {
		if m := ResolveFeatureModel(cfg, families, key, "claude"); m != "" {
			out += " --model " + m
		}
	}
	if !strings.Contains(base, "--effort") {
		if e := cfg.ModelPolicyValue(config.ModelPolicyBackgroundEffort); e != "" {
			out += " --effort " + e
		}
	}
	if out == base {
		return ""
	}
	return out
}

func isClaudeProgramString(program string) bool {
	for _, tok := range strings.Fields(program) {
		if tok == "claude" || strings.HasSuffix(tok, "/claude") {
			return true
		}
		if !strings.Contains(tok, "=") && tok != "env" && !strings.HasPrefix(tok, "-") {
			return false
		}
	}
	return false
}
