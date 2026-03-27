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

	"github.com/fsnotify/fsnotify"
	"github.com/tstapler/stapler-squad/log"
)

// RuleSpec is the JSON-serializable form of a Rule.
// CommandPattern and FilePattern are stored as strings (compiled on load).
type RuleSpec struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	ToolName       string    `json:"tool_name,omitempty"`
	ToolPattern    string    `json:"tool_pattern,omitempty"`
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

// RulesStore manages user-defined rules persisted to disk.
// Thread-safe for concurrent reads.
type RulesStore struct {
	mu       sync.RWMutex
	filePath string
	specs    []RuleSpec
}

// NewRulesStore creates a RulesStore backed by the given file path.
// If the file does not exist, an empty store is returned (no error).
func NewRulesStore(filePath string) (*RulesStore, error) {
	s := &RulesStore{filePath: filePath}
	if err := s.reload(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("rules_store: load %s: %w", filePath, err)
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
func (s *RulesStore) ToRules() []Rule {
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
	if err := s.saveLocked(); err != nil {
		return RuleSpec{}, fmt.Errorf("save rules: %w", err)
	}
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
			s.specs = append(s.specs[:i], s.specs[i+1:]...)
			return s.saveLocked()
		}
	}
	return fmt.Errorf("rule %q not found", id)
}

// WatchAndReload starts a goroutine that reloads rules when the file changes.
// The goroutine exits when ctx is canceled.
func (s *RulesStore) WatchAndReload(ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.WarningLog.Printf("[RulesStore] Failed to create watcher: %v", err)
		return
	}

	// Watch parent directory so we catch atomic renames.
	dir := filepath.Dir(s.filePath)
	if err := watcher.Add(dir); err != nil {
		log.WarningLog.Printf("[RulesStore] Failed to watch %s: %v", dir, err)
		watcher.Close()
		return
	}

	go func() {
		defer watcher.Close()
		debounce := time.NewTimer(0)
		if !debounce.Stop() {
			<-debounce.C
		}
		pending := false

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Name == s.filePath {
					if !pending {
						debounce.Reset(100 * time.Millisecond)
						pending = true
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.WarningLog.Printf("[RulesStore] Watcher error: %v", err)
			case <-debounce.C:
				pending = false
				before := len(s.All())
				if err := s.reload(); err != nil {
					log.WarningLog.Printf("[RulesStore] Reload failed, keeping previous rules: %v", err)
				} else {
					after := len(s.All())
					log.InfoLog.Printf("[RulesStore] Reloaded rules from %s (%d → %d rules)", s.filePath, before, after)
				}
			}
		}
	}()
}

// reload reads the rules file and updates the in-memory slice.
// Caller must NOT hold mu; reload acquires a write lock internally.
func (s *RulesStore) reload() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	var rf RulesFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return fmt.Errorf("parse %s: %w", s.filePath, err)
	}
	s.mu.Lock()
	s.specs = rf.Rules
	s.mu.Unlock()
	return nil
}

// saveLocked writes specs to disk with an atomic rename. Must be called with mu held (write lock).
func (s *RulesStore) saveLocked() error {
	rf := RulesFile{Version: 1, Rules: s.specs}
	data, err := json.MarshalIndent(rf, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := s.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.filePath)
}

// specsToRules compiles RuleSpec patterns into Rule structs.
// Specs with invalid regex are skipped with a warning log.
func specsToRules(specs []RuleSpec) []Rule {
	rules := make([]Rule, 0, len(specs))
	for _, spec := range specs {
		r := Rule{
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
				log.WarningLog.Printf("[RulesStore] Skipping rule %q: invalid tool_pattern %q: %v", spec.ID, spec.ToolPattern, err)
				continue
			}
			r.ToolPattern = re
		}
		if spec.CommandPattern != "" {
			re, err := regexp.Compile(spec.CommandPattern)
			if err != nil {
				log.WarningLog.Printf("[RulesStore] Skipping rule %q: invalid command_pattern %q: %v", spec.ID, spec.CommandPattern, err)
				continue
			}
			r.CommandPattern = re
		}
		if spec.FilePattern != "" {
			re, err := regexp.Compile(spec.FilePattern)
			if err != nil {
				log.WarningLog.Printf("[RulesStore] Skipping rule %q: invalid file_pattern %q: %v", spec.ID, spec.FilePattern, err)
				continue
			}
			r.FilePattern = re
		}
		rules = append(rules, r)
	}
	return rules
}

func parseDecision(s string) ClassificationDecision {
	switch s {
	case "auto_allow":
		return AutoAllow
	case "auto_deny":
		return AutoDeny
	default:
		return Escalate
	}
}

func parseRiskLevel(s string) RiskLevel {
	switch s {
	case "low":
		return RiskLow
	case "medium":
		return RiskMedium
	case "high":
		return RiskHigh
	case "critical":
		return RiskCritical
	default:
		return RiskMedium
	}
}
