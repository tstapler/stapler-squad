package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
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

	// Structured CommandCriteria fields — mutually exclusive with commandPattern.
	Programs              []string `json:"programs,omitempty"`
	Subcommands           []string `json:"subcommands,omitempty"`
	BlockedSubcommands    []string `json:"blocked_subcommands,omitempty"`
	RequiredFlags         []string `json:"required_flags,omitempty"`
	ForbiddenFlags        []string `json:"forbidden_flags,omitempty"`
	RequiredFlagPrefixes  []string `json:"required_flag_prefixes,omitempty"`
	PythonModes           []string `json:"python_modes,omitempty"`
	SafePythonImportsOnly bool     `json:"safe_python_imports_only,omitempty"`
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
	// Structured criteria and raw commandPattern are mutually exclusive.
	hasCriteria := len(spec.Programs) > 0 || len(spec.Subcommands) > 0 ||
		len(spec.BlockedSubcommands) > 0 || len(spec.RequiredFlags) > 0 ||
		len(spec.ForbiddenFlags) > 0 || len(spec.RequiredFlagPrefixes) > 0 ||
		len(spec.PythonModes) > 0 || spec.SafePythonImportsOnly
	if hasCriteria && spec.CommandPattern != "" {
		return RuleSpec{}, fmt.Errorf("rule cannot set both commandPattern and structured criteria; use one mode")
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

		Programs:              spec.Programs,
		Subcommands:           spec.Subcommands,
		BlockedSubcommands:    spec.BlockedSubcommands,
		RequiredFlags:         spec.RequiredFlags,
		ForbiddenFlags:        spec.ForbiddenFlags,
		RequiredFlagPrefixes:  spec.RequiredFlagPrefixes,
		PythonModes:           spec.PythonModes,
		SafePythonImportsOnly: spec.SafePythonImportsOnly,
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

			Programs:              r.Programs,
			Subcommands:           r.Subcommands,
			BlockedSubcommands:    r.BlockedSubcommands,
			RequiredFlags:         r.RequiredFlags,
			ForbiddenFlags:        r.ForbiddenFlags,
			RequiredFlagPrefixes:  r.RequiredFlagPrefixes,
			PythonModes:           r.PythonModes,
			SafePythonImportsOnly: r.SafePythonImportsOnly,
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
		// Populate Criteria from structured fields when at least one is set.
		if len(spec.Programs) > 0 ||
			len(spec.Subcommands) > 0 ||
			len(spec.BlockedSubcommands) > 0 ||
			len(spec.RequiredFlags) > 0 ||
			len(spec.ForbiddenFlags) > 0 ||
			len(spec.RequiredFlagPrefixes) > 0 ||
			len(spec.PythonModes) > 0 ||
			spec.SafePythonImportsOnly {
			r.Criteria = &classifier.CommandCriteria{
				Programs:              spec.Programs,
				Subcommands:           spec.Subcommands,
				BlockedSubcommands:    spec.BlockedSubcommands,
				RequiredFlags:         spec.RequiredFlags,
				ForbiddenFlags:        spec.ForbiddenFlags,
				RequiredFlagPrefixes:  spec.RequiredFlagPrefixes,
				PythonModes:           spec.PythonModes,
				SafePythonImportsOnly: spec.SafePythonImportsOnly,
			}
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

// decisionStringFromInt delegates to the canonical decisionString in analytics_store.go.
func decisionStringFromInt(d int) string {
	return decisionString(classifier.ClassificationDecision(d))
}

// riskLevelStringFromInt delegates to the canonical riskLevelString in analytics_store.go.
func riskLevelStringFromInt(r int) string {
	return riskLevelString(classifier.RiskLevel(r))
}

// ── BulkUpsert ────────────────────────────────────────────────────────────────

// BulkUpsertResult holds the outcome counts for a BulkUpsert operation.
type BulkUpsertResult struct {
	Created int
	Updated int
	Skipped int
	Errors  []string
}

// BulkUpsert creates or updates multiple user rules in one transaction.
// If overwriteDuplicates is false, rules whose name already exists are skipped.
// If overwriteDuplicates is true, rules whose name already exists are updated.
// This method does NOT call rebuildClassifier -- that is RulesService's responsibility.
// Lock ordering: BulkUpsert holds s.mu.Lock for the full operation, then calls
// s.storage.UpsertRule while holding the lock. This is safe because storage is
// independent of the in-memory lock (no recursive locking).
func (s *RulesStore) BulkUpsert(ctx context.Context, specs []RuleSpec, overwriteDuplicates bool) BulkUpsertResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Build name index for O(1) duplicate detection.
	nameIndex := make(map[string]string, len(s.specs)) // name -> id
	for _, existing := range s.specs {
		nameIndex[existing.Name] = existing.ID
	}

	result := BulkUpsertResult{}

	// Pre-flight: validate all specs before touching storage.
	// This catches the most common errors (bad data) before any writes occur.
	for i := range specs {
		if strings.TrimSpace(specs[i].Name) == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("rule at index %d: name is required", i))
		}
	}
	if len(result.Errors) > 0 {
		return result
	}

	for i := range specs {
		spec := specs[i]
		existingID, isDuplicate := nameIndex[spec.Name]
		if isDuplicate && !overwriteDuplicates {
			result.Skipped++
			continue
		}
		if isDuplicate {
			// Overwrite: preserve the existing ID.
			spec.ID = existingID
			result.Updated++
		} else {
			// New rule: assign server-generated ID.
			spec.ID = "user-" + uuid.New().String()
			result.Created++
		}
		spec.Source = "user"
		if spec.CreatedAt.IsZero() {
			spec.CreatedAt = time.Now()
		}

		ruleData := session.ApprovalRuleData{
			ID:                    spec.ID,
			Name:                  spec.Name,
			ToolName:              spec.ToolName,
			ToolPattern:           spec.ToolPattern,
			ToolCategory:          spec.ToolCategory,
			CommandPattern:        spec.CommandPattern,
			FilePattern:           spec.FilePattern,
			Decision:              decisionToInt(spec.Decision),
			RiskLevel:             riskLevelToInt(spec.RiskLevel),
			Reason:                spec.Reason,
			Alternative:           spec.Alternative,
			Priority:              spec.Priority,
			Enabled:               spec.Enabled,
			Source:                spec.Source,
			CreatedAt:             spec.CreatedAt,
			UpdatedAt:             time.Now(),
			Programs:              spec.Programs,
			Subcommands:           spec.Subcommands,
			BlockedSubcommands:    spec.BlockedSubcommands,
			RequiredFlags:         spec.RequiredFlags,
			ForbiddenFlags:        spec.ForbiddenFlags,
			RequiredFlagPrefixes:  spec.RequiredFlagPrefixes,
			PythonModes:           spec.PythonModes,
			SafePythonImportsOnly: spec.SafePythonImportsOnly,
		}

		if err := s.storage.UpsertRule(ctx, ruleData); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("rule %q: %v", spec.Name, err))
			if isDuplicate {
				result.Updated--
			} else {
				result.Created--
			}
			continue
		}

		// Update name index so subsequent rules in this batch see the new name.
		nameIndex[spec.Name] = spec.ID

		// Update in-memory slice.
		found := false
		for j, r := range s.specs {
			if r.ID == spec.ID {
				s.specs[j] = spec
				found = true
				break
			}
		}
		if !found {
			s.specs = append(s.specs, spec)
		}
	}

	// Write the JSON export file once at the end (not per-rule).
	s.exportRulesLocked()
	return result
}
