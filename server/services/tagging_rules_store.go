package services

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/session"
)

// ErrTaggingRuleNotFound is returned by Delete when no rule with the given ID exists,
// so RPC handlers can distinguish "not found" (CodeNotFound) from a genuine storage
// failure (CodeInternal) via errors.Is.
var ErrTaggingRuleNotFound = errors.New("tagging rule not found")

// ErrTaggingRuleValidation wraps every error validateTaggingRuleSpec returns, so callers
// can distinguish a rejected (bad-regex/missing-field) spec (CodeInvalidArgument) from a
// downstream persistence failure (CodeInternal) via errors.Is.
var ErrTaggingRuleValidation = errors.New("tagging rule validation failed")

// TaggingRuleSpec is the JSON-serializable form of a classifier.TaggingRule.
// Pattern fields are stored as strings (compiled on load), mirroring RuleSpec's convention.
type TaggingRuleSpec struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	NamePattern    string    `json:"name_pattern,omitempty"`
	BranchPattern  string    `json:"branch_pattern,omitempty"`
	PathPattern    string    `json:"path_pattern,omitempty"`
	ProgramPattern string    `json:"program_pattern,omitempty"`
	RequiredTags   []string  `json:"required_tags,omitempty"`
	OutputTag      string    `json:"output_tag"`
	Priority       int       `json:"priority"`
	Enabled        bool      `json:"enabled"`
	Source         string    `json:"source"` // "user" | "seed"
	CreatedAt      time.Time `json:"created_at"`
}

// TaggingRulesFile is the top-level structure of a tagging-rules export file.
type TaggingRulesFile struct {
	Version int               `json:"version"`
	Rules   []TaggingRuleSpec `json:"rules"`
}

// TaggingRulesStore manages user-defined tagging rules persisted to SQLite.
// Thread-safe for concurrent reads. Sibling of RulesStore.
type TaggingRulesStore struct {
	mu      sync.RWMutex
	storage *session.Storage
	specs   []TaggingRuleSpec
}

// NewTaggingRulesStore creates a TaggingRulesStore backed by the given storage.
func NewTaggingRulesStore(storage *session.Storage) (*TaggingRulesStore, error) {
	s := &TaggingRulesStore{storage: storage}
	if err := s.reload(); err != nil {
		return nil, fmt.Errorf("tagging_rules_store: load from storage: %w", err)
	}
	return s, nil
}

// All returns tagging rule specs.
func (s *TaggingRulesStore) All() []TaggingRuleSpec {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TaggingRuleSpec, len(s.specs))
	copy(out, s.specs)
	return out
}

// ToRules converts specs to compiled classifier.TaggingRules, skipping specs with
// invalid regex (logged, not fatal — mirrors RulesStore.ToRules/specsToRules).
func (s *TaggingRulesStore) ToRules() []classifier.TaggingRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return taggingSpecsToRules(s.specs)
}

// validateTaggingRuleSpec checks the required fields and every pattern's regex
// compile-ability before Upsert persists anything (Story 2.3.1's second acceptance
// criterion: an invalid regex must be rejected without touching storage).
func validateTaggingRuleSpec(spec TaggingRuleSpec) error {
	if spec.ID == "" {
		return fmt.Errorf("%w: rule ID is required", ErrTaggingRuleValidation)
	}
	if spec.OutputTag == "" {
		return fmt.Errorf("%w: output tag is required", ErrTaggingRuleValidation)
	}
	for _, pat := range []string{spec.NamePattern, spec.BranchPattern, spec.PathPattern, spec.ProgramPattern} {
		if pat == "" {
			continue
		}
		if _, err := regexp.Compile(pat); err != nil {
			return fmt.Errorf("%w: invalid regex %q: %v", ErrTaggingRuleValidation, pat, err)
		}
	}
	return nil
}

// Upsert creates or updates a tagging rule. Validates every pattern compiles before
// persisting anything.
func (s *TaggingRulesStore) Upsert(ctx context.Context, spec TaggingRuleSpec) (TaggingRuleSpec, error) {
	if err := validateTaggingRuleSpec(spec); err != nil {
		return TaggingRuleSpec{}, err
	}

	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = time.Now()
	}

	ruleData := session.TaggingRuleData{
		RuleID:         spec.ID,
		Name:           spec.Name,
		NamePattern:    spec.NamePattern,
		BranchPattern:  spec.BranchPattern,
		PathPattern:    spec.PathPattern,
		ProgramPattern: spec.ProgramPattern,
		RequiredTags:   spec.RequiredTags,
		OutputTag:      spec.OutputTag,
		Priority:       spec.Priority,
		Enabled:        spec.Enabled,
		Source:         spec.Source,
		CreatedAt:      spec.CreatedAt,
		UpdatedAt:      time.Now(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Persist to SQLite first; only update in-memory state on success.
	if err := s.storage.UpsertTaggingRule(ctx, ruleData); err != nil {
		return TaggingRuleSpec{}, fmt.Errorf("save tagging rule to DB: %w", err)
	}

	found := false
	for i, r := range s.specs {
		if r.ID == spec.ID {
			s.specs[i] = spec
			found = true
			break
		}
	}
	if !found {
		s.specs = append(s.specs, spec)
	}

	return spec, nil
}

// Delete removes a tagging rule by ID.
func (s *TaggingRulesStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, r := range s.specs {
		if r.ID == id {
			if err := s.storage.DeleteTaggingRule(ctx, id); err != nil {
				return fmt.Errorf("delete tagging rule from DB: %w", err)
			}
			s.specs = append(s.specs[:i], s.specs[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("tagging rule %q: %w", id, ErrTaggingRuleNotFound)
}

// WatchAndReload is a no-op — we use the shared DB, mirroring RulesStore's identical method.
func (s *TaggingRulesStore) WatchAndReload(ctx context.Context) {
}

// reload reads tagging rules from DB and updates the in-memory slice.
func (s *TaggingRulesStore) reload() error {
	rules, err := s.storage.AllTaggingRules(context.Background())
	if err != nil {
		return err
	}

	specs := make([]TaggingRuleSpec, len(rules))
	for i, r := range rules {
		specs[i] = TaggingRuleSpec{
			ID:             r.RuleID,
			Name:           r.Name,
			NamePattern:    r.NamePattern,
			BranchPattern:  r.BranchPattern,
			PathPattern:    r.PathPattern,
			ProgramPattern: r.ProgramPattern,
			RequiredTags:   r.RequiredTags,
			OutputTag:      r.OutputTag,
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

// taggingSpecsToRules compiles TaggingRuleSpec patterns into classifier.TaggingRule structs.
// Specs with invalid regex are skipped with a warning log, mirroring specsToRules.
func taggingSpecsToRules(specs []TaggingRuleSpec) []classifier.TaggingRule {
	rules := make([]classifier.TaggingRule, 0, len(specs))
	for _, spec := range specs {
		r := classifier.TaggingRule{
			RuleMeta: classifier.RuleMeta{
				ID:       spec.ID,
				Name:     spec.Name,
				Priority: spec.Priority,
				Enabled:  spec.Enabled,
				Source:   spec.Source,
			},
			RequiredTags: spec.RequiredTags,
			OutputTag:    spec.OutputTag,
		}

		skip := false
		compilePattern := func(name, pattern string, assign func(*regexp.Regexp)) {
			if pattern == "" || skip {
				return
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				log.Warn("[TaggingRulesStore] skipping rule: invalid pattern", "id", spec.ID, "field", name, "pattern", pattern, "err", err)
				skip = true
				return
			}
			assign(re)
		}
		compilePattern("name_pattern", spec.NamePattern, func(re *regexp.Regexp) { r.NamePattern = re })
		compilePattern("branch_pattern", spec.BranchPattern, func(re *regexp.Regexp) { r.BranchPattern = re })
		compilePattern("path_pattern", spec.PathPattern, func(re *regexp.Regexp) { r.PathPattern = re })
		compilePattern("program_pattern", spec.ProgramPattern, func(re *regexp.Regexp) { r.ProgramPattern = re })
		if skip {
			continue
		}

		rules = append(rules, r)
	}
	return rules
}
