package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/session"
)

// RuleSpec is the JSON-serializable form of a Rule.
// CommandPattern and FilePattern are stored as strings (compiled on load).
type RuleSpec struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	ToolName       string    `json:"tool_name,omitempty"`
	ToolPattern    string    `json:"tool_pattern,omitempty"`
	ToolCategory   string    `json:"tool_category,omitempty"`
	CommandPattern string    `json:"command_pattern,omitempty"`
	FilePattern    string    `json:"file_pattern,omitempty"`
	Decision       string    `json:"decision"`   // "auto_allow" | "auto_deny" | "escalate"
	RiskLevel      string    `json:"risk_level"` // "low" | "medium" | "high" | "critical"
	Reason         string    `json:"reason,omitempty"`
	Alternative    string    `json:"alternative,omitempty"`
	Priority       int       `json:"priority"`
	Enabled        bool      `json:"enabled"`
	Source         string    `json:"source"` // "user" | "seed" | "claude-settings"
	CreatedAt      time.Time `json:"created_at"`
}

// RulesFile is the top-level structure of auto_approve_rules.json.
type RulesFile struct {
	Version int        `json:"version"`
	Rules   []RuleSpec `json:"rules"`
}

// RulesStore manages user-defined rules persisted to SQLite.
// Thread-safe for concurrent reads.
type RulesStore struct {
	mu      sync.RWMutex
	storage *session.Storage
	specs   []RuleSpec
}

// NewRulesStore creates a RulesStore backed by the given storage.
func NewRulesStore(storage *session.Storage) (*RulesStore, error) {
	s := &RulesStore{storage: storage}
	if err := s.reload(); err != nil {
		return nil, fmt.Errorf("rules_store: load from storage: %w", err)
	}
	return s, nil
}

// All returns user rules as compiled Rules (source="user" only).
func (s *RulesStore) All() []RuleSpec {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RuleSpec, len(s.specs))
	copy(out, s.specs)
	return out
}

// ToRules converts specs to compiled Rules, skipping specs with invalid regex.
func (s *RulesStore) ToRules() []classifier.Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return specsToRules(s.specs)
}

// Upsert creates or updates a user rule. Source must be "user".
// Returns the upserted spec.
func (s *RulesStore) Upsert(spec RuleSpec) (RuleSpec, error) {
	if spec.Source != "user" {
		return RuleSpec{}, fmt.Errorf("only user rules can be modified; got source=%q", spec.Source)
	}
	if spec.ID == "" {
		return RuleSpec{}, fmt.Errorf("rule ID is required")
	}
	// Validate patterns.
	for _, pat := range []string{spec.ToolPattern, spec.CommandPattern, spec.FilePattern} {
		if pat != "" {
			if _, err := regexp.Compile(pat); err != nil {
				return RuleSpec{}, fmt.Errorf("invalid regex %q: %w", pat, err)
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	found := false
	for i, r := range s.specs {
		if r.ID == spec.ID {
			s.specs[i] = spec
			found = true
			break
		}
	}
	if !found {
		if spec.CreatedAt.IsZero() {
			spec.CreatedAt = time.Now()
		}
		s.specs = append(s.specs, spec)
	}

	// Persist to SQLite via Storage.
	ruleData := session.ApprovalRuleData{
		ID:             spec.ID,
		Name:           spec.Name,
		ToolName:       spec.ToolName,
		ToolPattern:    spec.ToolPattern,
		ToolCategory:   spec.ToolCategory,
		CommandPattern: spec.CommandPattern,
		FilePattern:    spec.FilePattern,
		Decision:       decisionToInt(spec.Decision),
		RiskLevel:      riskLevelToInt(spec.RiskLevel),
		Reason:         spec.Reason,
		Alternative:    spec.Alternative,
		Priority:       spec.Priority,
		Enabled:        spec.Enabled,
		Source:         spec.Source,
		CreatedAt:      spec.CreatedAt,
		UpdatedAt:      time.Now(),
	}

	if err := s.storage.UpsertRule(context.Background(), ruleData); err != nil {
		return RuleSpec{}, fmt.Errorf("save rule to DB: %w", err)
	}

	s.exportRulesLocked()
	return spec, nil
}

// Delete removes a user rule by ID. Returns error if not found or not a user rule.
func (s *RulesStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, r := range s.specs {
		if r.ID == id {
			if r.Source != "user" {
				return fmt.Errorf("cannot delete %q rule %q; only user rules can be deleted", r.Source, id)
			}

			if err := s.storage.DeleteRule(context.Background(), id); err != nil {
				return fmt.Errorf("delete rule from DB: %w", err)
			}

			s.specs = append(s.specs[:i], s.specs[i+1:]...)
			s.exportRulesLocked()
			return nil
		}
	}
	return fmt.Errorf("rule %q not found", id)
}

// WatchAndReload is now a no-op as we use shared DB.
func (s *RulesStore) WatchAndReload(ctx context.Context) {
}

// reload reads rules from DB and updates the in-memory slice.
func (s *RulesStore) reload() error {
	rules, err := s.storage.AllRules(context.Background())
	if err != nil {
		return err
	}

	specs := make([]RuleSpec, len(rules))
	for i, r := range rules {
		specs[i] = RuleSpec{
			ID:             r.ID,
			Name:           r.Name,
			ToolName:       r.ToolName,
			ToolPattern:    r.ToolPattern,
			ToolCategory:   r.ToolCategory,
			CommandPattern: r.CommandPattern,
			FilePattern:    r.FilePattern,
			Decision:       decisionStringFromInt(r.Decision),
			RiskLevel:      riskLevelStringFromInt(r.RiskLevel),
			Reason:         r.Reason,
			Alternative:    r.Alternative,
			Priority:       r.Priority,
			Enabled:        r.Enabled,
			Source:         r.Source,
			CreatedAt:      r.CreatedAt,
		}
	}

	s.mu.Lock()
	s.specs = specs
	s.mu.Unlock()
	return nil
}

// exportRulesLocked writes rule specs to ~/.config/stapler-squad/rules.json
// for use by standalone hooks. Exports the serializable RuleSpec structs
// (not compiled Rules, which contain *regexp.Regexp that won't round-trip
// through JSON). Errors are logged but not returned to avoid blocking
// main application storage.
func (s *RulesStore) exportRulesLocked() {
	home, _ := os.UserHomeDir()
	exportPath := filepath.Join(home, ".config", "stapler-squad", "rules.json")

	data, err := json.MarshalIndent(s.specs, "", "  ")
	if err != nil {
		return
	}

	exportDir := filepath.Dir(exportPath)
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		return
	}

	tmp := exportPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return
	}
	if err := os.Rename(tmp, exportPath); err != nil {
		os.Remove(tmp)
	}
}

// specsToRules compiles RuleSpec patterns into Rule structs.
// Specs with invalid regex are skipped with a warning log.
func specsToRules(specs []RuleSpec) []classifier.Rule {
	rules := make([]classifier.Rule, 0, len(specs))
	for _, spec := range specs {
		r := classifier.Rule{
			ID:          spec.ID,
			Name:        spec.Name,
			ToolName:    spec.ToolName,
			Decision:    parseDecision(spec.Decision),
			RiskLevel:   parseRiskLevel(spec.RiskLevel),
			Reason:      spec.Reason,
			Alternative: spec.Alternative,
			Priority:    spec.Priority,
			Enabled:     spec.Enabled,
			Source:      spec.Source,
		}
		if spec.ToolPattern != "" {
			re, err := regexp.Compile(spec.ToolPattern)
			if err != nil {
				log.Warn("[RulesStore] skipping rule: invalid tool_pattern", "id", spec.ID, "pattern", spec.ToolPattern, "err", err)
				continue
			}
			r.ToolPattern = re
		}
		if spec.CommandPattern != "" {
			re, err := regexp.Compile(spec.CommandPattern)
			if err != nil {
				log.Warn("[RulesStore] skipping rule: invalid command_pattern", "id", spec.ID, "pattern", spec.CommandPattern, "err", err)
				continue
			}
			r.CommandPattern = re
		}
		if spec.FilePattern != "" {
			re, err := regexp.Compile(spec.FilePattern)
			if err != nil {
				log.Warn("[RulesStore] skipping rule: invalid file_pattern", "id", spec.ID, "pattern", spec.FilePattern, "err", err)
				continue
			}
			r.FilePattern = re
		}
		rules = append(rules, r)
	}
	return rules
}

func parseDecision(s string) classifier.ClassificationDecision {
	switch s {
	case "auto_allow":
		return classifier.AutoAllow
	case "auto_deny":
		return classifier.AutoDeny
	default:
		return classifier.Escalate
	}
}

func parseRiskLevel(s string) classifier.RiskLevel {
	switch s {
	case "low":
		return classifier.RiskLow
	case "medium":
		return classifier.RiskMedium
	case "high":
		return classifier.RiskHigh
	case "critical":
		return classifier.RiskCritical
	default:
		return classifier.RiskMedium
	}
}

func decisionToInt(s string) int {
	switch s {
	case "auto_allow":
		return int(classifier.AutoAllow)
	case "auto_deny":
		return int(classifier.AutoDeny)
	default:
		return int(classifier.Escalate)
	}
}

func riskLevelToInt(s string) int {
	switch s {
	case "low":
		return int(classifier.RiskLow)
	case "medium":
		return int(classifier.RiskMedium)
	case "high":
		return int(classifier.RiskHigh)
	case "critical":
		return int(classifier.RiskCritical)
	default:
		return int(classifier.RiskMedium)
	}
}

func decisionStringFromInt(d int) string {
	switch classifier.ClassificationDecision(d) {
	case classifier.AutoAllow:
		return "auto_allow"
	case classifier.AutoDeny:
		return "auto_deny"
	default:
		return "escalate"
	}
}

func riskLevelStringFromInt(r int) string {
	switch classifier.RiskLevel(r) {
	case classifier.RiskLow:
		return "low"
	case classifier.RiskMedium:
		return "medium"
	case classifier.RiskHigh:
		return "high"
	case classifier.RiskCritical:
		return "critical"
	default:
		return "medium"
	}
}
