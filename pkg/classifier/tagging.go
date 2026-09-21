package classifier

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/linkdata/deadlock"
)

// TaggingRule matches a session's tagging-relevant attributes and, when it matches,
// contributes a single OutputTag. It shares RuleMeta (ID/Name/Priority/Enabled/Source) with
// Rule but never leaks tool-use fields — the tagging domain is deliberately kept separate.
//
// A nil pattern field means "any value matches", mirroring Rule.CommandPattern's convention.
type TaggingRule struct {
	RuleMeta
	// NamePattern matches against SessionTaggingContext.Name. nil means any name matches.
	NamePattern *regexp.Regexp
	// BranchPattern matches against SessionTaggingContext.Branch. nil means any branch matches.
	BranchPattern *regexp.Regexp
	// PathPattern matches against SessionTaggingContext.Path. nil means any path matches.
	PathPattern *regexp.Regexp
	// ProgramPattern matches against SessionTaggingContext.Program. nil means any program matches.
	ProgramPattern *regexp.Regexp
	// RequiredTags: the rule only matches once the session already carries every tag listed
	// here — this is how tag-dependency chains are expressed (see TaggingEngine.ApplyToFixpoint).
	RequiredTags []string
	// OutputTag is the single tag this rule contributes when it matches.
	OutputTag string
}

// SessionTaggingContext is the minimal input matchesTaggingRule evaluates against. It is
// deliberately decoupled from the session package (which already imports pkg/classifier) —
// adding that import here would create an import cycle.
type SessionTaggingContext struct {
	Name    string
	Branch  string
	Path    string
	Program string
	Tags    []string
}

// matchesTaggingRule returns true if rule is enabled, every non-nil pattern matches its
// corresponding ctx field, and every entry in rule.RequiredTags is already present in
// ctx.Tags.
func matchesTaggingRule(rule TaggingRule, ctx SessionTaggingContext) bool {
	if !rule.Enabled {
		return false
	}
	if rule.NamePattern != nil && !rule.NamePattern.MatchString(ctx.Name) {
		return false
	}
	if rule.BranchPattern != nil && !rule.BranchPattern.MatchString(ctx.Branch) {
		return false
	}
	if rule.PathPattern != nil && !rule.PathPattern.MatchString(ctx.Path) {
		return false
	}
	if rule.ProgramPattern != nil && !rule.ProgramPattern.MatchString(ctx.Program) {
		return false
	}
	for _, required := range rule.RequiredTags {
		if !slices.Contains(ctx.Tags, required) {
			return false
		}
	}
	return true
}

// TagMatch pairs a produced tag with the exact rule that matched it. Attribution is always
// taken directly from the evaluation that produced it — never reconstructed afterward via
// OutputTag string equality, which would misattribute a tag to a higher-priority rule that
// merely happens to share the same OutputTag but whose own condition never held.
type TagMatch struct {
	Tag    string
	RuleID string
}

// maxTaggingFixpointIterations bounds ApplyToFixpoint's loop so a pathological or
// deliberately deep tag-dependency chain always terminates deterministically instead of
// looping unboundedly.
const maxTaggingFixpointIterations = 10

// TaggingEngine holds a priority-ordered set of TaggingRules and evaluates them against a
// SessionTaggingContext. Mirrors RuleBasedClassifier's CRUD-mutation shape so the
// persistence layer (Phase 2) can reuse the same rebuild primitives.
type TaggingEngine struct {
	mu    deadlock.RWMutex
	rules []TaggingRule // sorted by Priority descending
}

// NewTaggingEngine creates an engine pre-loaded with the seed tagging rules.
func NewTaggingEngine() *TaggingEngine {
	rules := SeedTaggingRules()
	slices.SortStableFunc(rules, byPriorityDescending)
	return &TaggingEngine{rules: rules}
}

// byPriorityDescending orders TaggingRules by Priority descending, stably (ties preserve
// input order) — using slices.SortStableFunc rather than sort.Slice is deliberate: an
// unstable sort would make tied-priority ordering flap between calls.
func byPriorityDescending(a, b TaggingRule) int {
	return b.Priority - a.Priority
}

// ReplaceRules atomically replaces all rules with the provided list, sorted by Priority
// descending (stable — ties preserve input order).
func (e *TaggingEngine) ReplaceRules(rules []TaggingRule) {
	sorted := make([]TaggingRule, len(rules))
	copy(sorted, rules)
	slices.SortStableFunc(sorted, byPriorityDescending)
	e.mu.Lock()
	e.rules = sorted
	e.mu.Unlock()
}

// AddRules appends additional rules and re-sorts by priority (stable).
func (e *TaggingEngine) AddRules(rules []TaggingRule) {
	e.mu.Lock()
	e.rules = append(e.rules, rules...)
	slices.SortStableFunc(e.rules, byPriorityDescending)
	e.mu.Unlock()
}

// Rules returns a copy of the current rule set.
func (e *TaggingEngine) Rules() []TaggingRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]TaggingRule, len(e.rules))
	copy(out, e.rules)
	return out
}

// EvalOnce evaluates every rule against ctx exactly once and returns the tags newly
// produced by this single pass — each paired with the exact rule that matched it. Tags
// already present in ctx.Tags are never re-added or re-attributed. Rules are evaluated in
// priority order (e.rules is kept sorted); if multiple matching rules in this same pass
// share an OutputTag, the first (highest-priority) actual match wins and later rules
// sharing that tag are skipped without evaluation.
//
// EvalOnce is single-pass and does not thread newly-added tags to later rules within the
// same call — that chaining happens across multiple calls in ApplyToFixpoint.
func (e *TaggingEngine) EvalOnce(ctx SessionTaggingContext) []TagMatch {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var matches []TagMatch
	for _, rule := range e.rules {
		if slices.Contains(ctx.Tags, rule.OutputTag) || containsTag(matches, rule.OutputTag) {
			continue
		}
		if matchesTaggingRule(rule, ctx) {
			matches = append(matches, TagMatch{Tag: rule.OutputTag, RuleID: rule.ID})
		}
	}
	return matches
}

// containsTag reports whether any TagMatch in matches already produced tag, so EvalOnce can
// dedup within a single pass without a separate map allocation on the common (few-rules) path.
func containsTag(matches []TagMatch, tag string) bool {
	for _, m := range matches {
		if m.Tag == tag {
			return true
		}
	}
	return false
}

// ApplyToFixpoint repeatedly calls EvalOnce, feeding each pass's newly-added tags back into
// the context for the next pass, until either a pass makes no progress (the common case — a
// true fixpoint) or maxTaggingFixpointIterations is reached (capHit == true). This is how
// tag-dependency chains (RequiredTags) resolve deterministically across multiple passes: a
// rule whose RequiredTags were just satisfied by the previous pass can only fire starting on
// the next one, since EvalOnce evaluates against a fixed snapshot of ctx.Tags.
//
// added collects every TagMatch produced across all iterations (a tag's RuleID is fixed at
// the iteration it was first matched). On cap-hit, stillChurning holds the tag values that
// were still newly matching on the final permitted iteration, so a caller can tell this was
// a real unresolved chain rather than a silent truncation.
func (e *TaggingEngine) ApplyToFixpoint(ctx SessionTaggingContext) (added []TagMatch, capHit bool, stillChurning []string) {
	tags := append([]string(nil), ctx.Tags...)

	for iteration := 0; iteration < maxTaggingFixpointIterations; iteration++ {
		roundCtx := ctx
		roundCtx.Tags = tags
		matches := e.EvalOnce(roundCtx)
		if len(matches) == 0 {
			return added, false, nil
		}
		added = append(added, matches...)
		for _, m := range matches {
			tags = append(tags, m.Tag)
		}
		if iteration == maxTaggingFixpointIterations-1 {
			stillChurning = make([]string, len(matches))
			for i, m := range matches {
				stillChurning[i] = m.Tag
			}
			return added, true, stillChurning
		}
	}
	return added, false, nil
}

// SeedTaggingRules returns the built-in tagging rule set, covering the most common branch
// naming conventions and program identity, plus one tag-dependency example demonstrating
// chaining (seed-tag-bugfix-needs-review fires only once "Bugfix" has already been applied).
// Tag names follow docs/reference/tag-organization.md's existing vocabulary (Frontend/Backend)
// where applicable, and this project's own branch/path conventions elsewhere.
func SeedTaggingRules() []TaggingRule {
	return []TaggingRule{
		{RuleMeta: seedTagMeta("seed-tag-bugfix", "Bugfix branch", 100), BranchPattern: regexp.MustCompile(`^(bugfix|fix)/`), OutputTag: "Bugfix"},
		{RuleMeta: seedTagMeta("seed-tag-feature", "Feature branch", 100), BranchPattern: regexp.MustCompile(`^(feature|feat)/`), OutputTag: "Feature"},
		{RuleMeta: seedTagMeta("seed-tag-hotfix", "Hotfix branch", 100), BranchPattern: regexp.MustCompile(`^hotfix/`), OutputTag: "Urgent"},
		{RuleMeta: seedTagMeta("seed-tag-claude", "Claude-driven session", 50), ProgramPattern: regexp.MustCompile(`^claude$`), OutputTag: "Claude"},
		{RuleMeta: seedTagMeta("seed-tag-frontend", "Frontend working directory", 50), PathPattern: regexp.MustCompile(`(^|/)web-app/`), OutputTag: "Frontend"},
		{RuleMeta: seedTagMeta("seed-tag-backend", "Backend working directory", 50), PathPattern: regexp.MustCompile(`(^|/)server/`), OutputTag: "Backend"},
		// Chaining example: any session already tagged Bugfix also gets flagged NeedsReview,
		// one fixpoint iteration after seed-tag-bugfix fires.
		{RuleMeta: seedTagMeta("seed-tag-bugfix-needs-review", "Bugfix needs review", 10), RequiredTags: []string{"Bugfix"}, OutputTag: "NeedsReview"},
	}
}

// TagContentHash hashes the fields of ctx that the Phase 4 LLM fallback poller uses to decide
// whether a session needs re-classification. Tags are sorted before hashing so two contexts
// with the same tag set in a different order hash identically — the poller's cache key and its
// prompt-scoping logic both call this one function so they can never silently desync
// (pitfalls.md #5d).
func TagContentHash(ctx SessionTaggingContext) string {
	sortedTags := append([]string(nil), ctx.Tags...)
	sort.Strings(sortedTags)
	joined := strings.Join([]string{ctx.Name, ctx.Branch, ctx.Path, ctx.Program, strings.Join(sortedTags, "\x00")}, "\x00")
	sum := sha256.Sum256([]byte(joined))
	return hex.EncodeToString(sum[:])
}

// seedTagMeta builds the RuleMeta shared by every SeedTaggingRules() entry: seed-sourced and
// enabled, per Story 1.4.1's invariant.
func seedTagMeta(id, name string, priority int) RuleMeta {
	return RuleMeta{ID: id, Name: name, Priority: priority, Enabled: true, Source: string(SourceSeed)}
}
