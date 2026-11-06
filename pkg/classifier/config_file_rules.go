package classifier

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// SourceConfig marks rules loaded from ~/.config/stapler-squad/shared_rules.yaml.
const SourceConfig RuleSource = "config"

// ConfigFileRulesPath returns the path of shared_rules.yaml under home.
func ConfigFileRulesPath(home string) string {
	return filepath.Join(home, ".config", "stapler-squad", "shared_rules.yaml")
}

// LoadConfigFileRules parses a shared_rules.yaml file into classifier rules. A missing file is
// not an error (nil, nil). It is the single parser shared by ssq-hooks' local path and the
// server's hot-reloaded classifier, so both paths always see the same rule set.
//
// A rule with an invalid regex is skipped, and one with an unknown decision becomes an
// escalate rule; both are reported in the returned error alongside the valid rules. Neither is
// ever widened into an allow. An unreadable, unparsable
// or empty file returns (nil, err) so callers can keep their last-good rules.
func LoadConfigFileRules(path string) ([]Rule, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is built from $HOME plus a fixed filename, not caller input.
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%s is empty (half-written save?)", path)
	}
	var file struct {
		Rules []configRuleSpec `yaml:"rules"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	rules := []Rule{} // non-nil: callers tell "parsed, zero rules" from a failed load
	var problems []error
	for _, spec := range file.Rules {
		if spec.Name == "" {
			continue
		}
		if !knownDecision(spec.Decision) {
			problems = append(problems, fmt.Errorf("rule %q: unknown decision %q, treated as escalate", spec.Name, spec.Decision))
		}
		rule, err := spec.toRule()
		if err != nil {
			problems = append(problems, fmt.Errorf("rule %q skipped: %w", spec.Name, err))
			continue
		}
		rules = append(rules, rule)
	}
	return rules, errors.Join(problems...)
}

type configRuleSpec struct {
	Name           string   `yaml:"name"`
	Tool           string   `yaml:"tool"`
	ToolPattern    string   `yaml:"tool_pattern"`
	Programs       []string `yaml:"programs"`
	Subcommands    []string `yaml:"subcommands"`
	BlockedSubs    []string `yaml:"blocked_subcommands"`
	CommandPattern string   `yaml:"command_pattern"`
	FilePattern    string   `yaml:"file_pattern"`
	Decision       string   `yaml:"decision"`
	Priority       int      `yaml:"priority"`
	Enabled        *bool    `yaml:"enabled"`
}

func knownDecision(d string) bool {
	switch d {
	case "", "escalate", "allow", "deny":
		return true
	}
	return false
}

// toRule converts a spec to a Rule, rejecting an invalid regex rather than widening the rule.
func (r configRuleSpec) toRule() (Rule, error) {
	enabled := r.Enabled == nil || *r.Enabled
	priority := r.Priority
	if priority == 0 {
		priority = 10
	}
	decision := Escalate
	switch r.Decision {
	case "", "escalate":
	case "allow":
		decision = AutoAllow
	case "deny":
		decision = AutoDeny
	default:
		// Unknown values keep guarding as Escalate, as the pre-shared parser did; the loader
		// reports them.
	}
	cr := Rule{
		ToolName: r.Tool,
		Decision: decision,
		RuleMeta: RuleMeta{ID: "config-" + strings.ReplaceAll(r.Name, " ", "-"), Name: r.Name, Priority: priority, Enabled: enabled, Source: string(SourceConfig)},
	}
	var err error
	if cr.ToolPattern, err = compilePattern("tool_pattern", r.ToolPattern); err != nil {
		return Rule{}, err
	}
	if cr.CommandPattern, err = compilePattern("command_pattern", r.CommandPattern); err != nil {
		return Rule{}, err
	}
	if cr.FilePattern, err = compilePattern("file_pattern", r.FilePattern); err != nil {
		return Rule{}, err
	}
	if len(r.Programs) > 0 || len(r.Subcommands) > 0 || len(r.BlockedSubs) > 0 {
		cr.Criteria = &CommandCriteria{Programs: r.Programs, Subcommands: r.Subcommands, BlockedSubcommands: r.BlockedSubs}
	}
	return cr, nil
}

// compilePattern returns a nil regexp for an unset (empty) pattern, which Rule treats as "no
// constraint".
func compilePattern(field, pattern string) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, nil //nolint:nilnil // nil means "pattern not set"; callers rely on it
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", field, err)
	}
	return compiled, nil
}
