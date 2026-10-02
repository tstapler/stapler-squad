package classifier

import (
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
func LoadConfigFileRules(path string) ([]Rule, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is built from $HOME plus a fixed filename, not caller input.
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var file struct {
		Rules []configRuleSpec `yaml:"rules"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	var rules []Rule
	for _, spec := range file.Rules {
		if spec.Name == "" {
			continue
		}
		rules = append(rules, spec.toRule())
	}
	return rules, nil
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

// toRule converts a spec to a Rule. Invalid regexes are dropped silently, matching the
// historical ssq-hooks behavior.
func (r configRuleSpec) toRule() Rule {
	enabled := r.Enabled == nil || *r.Enabled
	priority := r.Priority
	if priority == 0 {
		priority = 10
	}
	decision := Escalate
	switch r.Decision {
	case "allow":
		decision = AutoAllow
	case "deny":
		decision = AutoDeny
	}
	cr := Rule{
		ToolName: r.Tool,
		Decision: decision,
		RuleMeta: RuleMeta{ID: "config-" + strings.ReplaceAll(r.Name, " ", "-"), Name: r.Name, Priority: priority, Enabled: enabled, Source: string(SourceConfig)},
	}
	cr.ToolPattern = compileOrNil(r.ToolPattern)
	cr.CommandPattern = compileOrNil(r.CommandPattern)
	cr.FilePattern = compileOrNil(r.FilePattern)
	if len(r.Programs) > 0 || len(r.Subcommands) > 0 || len(r.BlockedSubs) > 0 {
		cr.Criteria = &CommandCriteria{Programs: r.Programs, Subcommands: r.Subcommands, BlockedSubcommands: r.BlockedSubs}
	}
	return cr
}

func compileOrNil(pattern string) *regexp.Regexp {
	if pattern == "" {
		return nil
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	return compiled
}
