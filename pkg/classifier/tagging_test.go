package classifier

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"testing"
)

// ── Story 1.1.2: TaggingRule type ──────────────────────────────────────────

func TestTaggingRule_should_ConstructWithOnlyRelevantFields_When_SingleFieldRuleAuthored(t *testing.T) {
	rule := TaggingRule{
		RuleMeta:      RuleMeta{ID: "r-bugfix", Name: "Bugfix branch", Priority: 50, Enabled: true, Source: "seed"},
		BranchPattern: regexp.MustCompile("^(bugfix|fix)/"),
		OutputTag:     "Bugfix",
	}

	if rule.NamePattern != nil {
		t.Errorf("NamePattern = %v, want nil", rule.NamePattern)
	}
	if rule.PathPattern != nil {
		t.Errorf("PathPattern = %v, want nil", rule.PathPattern)
	}
	if rule.ProgramPattern != nil {
		t.Errorf("ProgramPattern = %v, want nil", rule.ProgramPattern)
	}
	if rule.RequiredTags != nil {
		t.Errorf("RequiredTags = %v, want nil", rule.RequiredTags)
	}
}

// ── Story 1.2.1: matchesTaggingRule ────────────────────────────────────────

func TestMatchesTaggingRule_should_ReturnTrue_When_BranchPatternMatchesAndOthersNil(t *testing.T) {
	rule := TaggingRule{RuleMeta: RuleMeta{Enabled: true}, BranchPattern: regexp.MustCompile("^bugfix/"), OutputTag: "Bugfix"}
	ctx := SessionTaggingContext{Branch: "bugfix/pr-poller"}

	if !matchesTaggingRule(rule, ctx) {
		t.Error("matchesTaggingRule() = false, want true")
	}
}

func TestMatchesTaggingRule_should_ReturnTrue_When_PathPatternMatchesAndOthersNil(t *testing.T) {
	rule := TaggingRule{RuleMeta: RuleMeta{Enabled: true}, PathPattern: regexp.MustCompile(`(^|/)web-app/`), OutputTag: "Frontend"}
	ctx := SessionTaggingContext{Path: "/repo/web-app/src"}

	if !matchesTaggingRule(rule, ctx) {
		t.Error("matchesTaggingRule() = false, want true")
	}
}

func TestMatchesTaggingRule_should_ReturnFalse_When_RequiredTagMissing(t *testing.T) {
	rule := TaggingRule{RuleMeta: RuleMeta{Enabled: true}, RequiredTags: []string{"Frontend"}, OutputTag: "NeedsReview"}

	ctx := SessionTaggingContext{Tags: []string{"Backend"}}
	if matchesTaggingRule(rule, ctx) {
		t.Error("matchesTaggingRule() = true, want false when required tag missing")
	}

	ctx.Tags = []string{"Frontend"}
	if !matchesTaggingRule(rule, ctx) {
		t.Error("matchesTaggingRule() = false, want true once required tag present")
	}
}

func TestMatchesTaggingRule_should_ReturnFalse_When_RuleDisabled(t *testing.T) {
	rule := TaggingRule{RuleMeta: RuleMeta{Enabled: false}, BranchPattern: regexp.MustCompile(".*"), OutputTag: "Bugfix"}
	ctx := SessionTaggingContext{Branch: "bugfix/anything"}

	if matchesTaggingRule(rule, ctx) {
		t.Error("matchesTaggingRule() = true, want false for a disabled rule")
	}
}

// ── Story 1.3.1: ReplaceRules/AddRules/Rules ───────────────────────────────

func TestReplaceRules_should_SortDescendingByPriority_When_RulesAdded(t *testing.T) {
	e := &TaggingEngine{}
	e.ReplaceRules([]TaggingRule{
		{RuleMeta: RuleMeta{ID: "a", Priority: 5}},
		{RuleMeta: RuleMeta{ID: "b", Priority: 10}},
	})

	rules := e.Rules()
	if len(rules) != 2 {
		t.Fatalf("len(Rules()) = %d, want 2", len(rules))
	}
	if rules[0].ID != "b" || rules[1].ID != "a" {
		t.Errorf("Rules() = [%s, %s], want [b, a]", rules[0].ID, rules[1].ID)
	}
}

func TestReplaceRules_should_PreserveInputOrder_When_PrioritiesTied(t *testing.T) {
	e := &TaggingEngine{}
	input := []TaggingRule{
		{RuleMeta: RuleMeta{ID: "c", Priority: 10}},
		{RuleMeta: RuleMeta{ID: "d", Priority: 10}},
	}

	for i := 0; i < 5; i++ {
		e.ReplaceRules(input)
		rules := e.Rules()
		if len(rules) != 2 || rules[0].ID != "c" || rules[1].ID != "d" {
			t.Fatalf("call %d: Rules() = %v, want [c, d] every time (stable sort)", i, rules)
		}
	}
}

// ── Story 1.3.2: EvalOnce ───────────────────────────────────────────────────

func TestEvalOnce_should_ReturnEmptyMatches_When_TagAlreadyPresent(t *testing.T) {
	e := &TaggingEngine{}
	e.ReplaceRules([]TaggingRule{
		{RuleMeta: RuleMeta{ID: "bugfix", Enabled: true}, BranchPattern: regexp.MustCompile("^bugfix/"), OutputTag: "Bugfix"},
	})
	ctx := SessionTaggingContext{Branch: "bugfix/x", Tags: []string{"Bugfix"}}

	matches := e.EvalOnce(ctx)
	if len(matches) != 0 {
		t.Errorf("EvalOnce() = %v, want no matches for an already-present tag", matches)
	}
}

func TestEvalOnce_should_AttributeTagToActualMatchingRule_When_TwoRulesShareOutputTag(t *testing.T) {
	e := &TaggingEngine{}
	ruleHi := TaggingRule{RuleMeta: RuleMeta{ID: "hi", Enabled: true, Priority: 100}, PathPattern: regexp.MustCompile("^/ui/"), OutputTag: "Frontend"}
	ruleLo := TaggingRule{RuleMeta: RuleMeta{ID: "lo", Enabled: true, Priority: 10}, BranchPattern: regexp.MustCompile("^frontend/"), OutputTag: "Frontend"}
	e.ReplaceRules([]TaggingRule{ruleHi, ruleLo})

	ctx := SessionTaggingContext{Branch: "frontend/x", Path: "/backend/y"}
	matches := e.EvalOnce(ctx)

	want := []TagMatch{{Tag: "Frontend", RuleID: "lo"}}
	if !slices.Equal(matches, want) {
		t.Errorf("EvalOnce() = %v, want %v (must credit the rule that actually matched, not the higher-priority one sharing OutputTag)", matches, want)
	}
}

// ── Story 1.3.2: ApplyToFixpoint ────────────────────────────────────────────

func TestApplyToFixpoint_should_ResolveTwoRuleDependencyChain_When_BranchMatchesInitialRule(t *testing.T) {
	e := &TaggingEngine{}
	ruleA := TaggingRule{RuleMeta: RuleMeta{ID: "a", Enabled: true}, OutputTag: "X", BranchPattern: regexp.MustCompile("^feat/")}
	ruleB := TaggingRule{RuleMeta: RuleMeta{ID: "b", Enabled: true}, OutputTag: "Y", RequiredTags: []string{"X"}}
	e.ReplaceRules([]TaggingRule{ruleA, ruleB})

	added, capHit, stillChurning := e.ApplyToFixpoint(SessionTaggingContext{Branch: "feat/foo"})

	if capHit {
		t.Errorf("capHit = true, want false")
	}
	if len(stillChurning) != 0 {
		t.Errorf("stillChurning = %v, want empty", stillChurning)
	}
	want := []TagMatch{{Tag: "X", RuleID: "a"}, {Tag: "Y", RuleID: "b"}}
	if !sameTagMatchSet(added, want) {
		t.Errorf("added = %v, want set %v", added, want)
	}
}

func TestApplyToFixpoint_should_ExitOnNoProgress_When_TwoRulesFormMutualCycle(t *testing.T) {
	e := &TaggingEngine{}
	ruleA := TaggingRule{RuleMeta: RuleMeta{ID: "a", Enabled: true}, OutputTag: "X", RequiredTags: []string{"Y"}}
	ruleB := TaggingRule{RuleMeta: RuleMeta{ID: "b", Enabled: true}, OutputTag: "Y", RequiredTags: []string{"X"}}
	e.ReplaceRules([]TaggingRule{ruleA, ruleB})

	added, capHit, stillChurning := e.ApplyToFixpoint(SessionTaggingContext{})

	if added != nil {
		t.Errorf("added = %v, want nil", added)
	}
	if capHit {
		t.Errorf("capHit = true, want false (should exit via no-progress, not the cap)")
	}
	if stillChurning != nil {
		t.Errorf("stillChurning = %v, want nil", stillChurning)
	}
}

func TestApplyToFixpoint_should_HitCapAndReportStillChurning_When_ChainExceedsMaxIterations(t *testing.T) {
	e := &TaggingEngine{}
	rules := make([]TaggingRule, 0, 11)
	rules = append(rules, TaggingRule{RuleMeta: RuleMeta{ID: "r0", Enabled: true}, OutputTag: "T0"})
	for i := 1; i <= 10; i++ {
		rules = append(rules, TaggingRule{
			RuleMeta:     RuleMeta{ID: fmt.Sprintf("r%d", i), Enabled: true},
			RequiredTags: []string{fmt.Sprintf("T%d", i-1)},
			OutputTag:    fmt.Sprintf("T%d", i),
		})
	}
	e.ReplaceRules(rules)

	added, capHit, stillChurning := e.ApplyToFixpoint(SessionTaggingContext{})

	if !capHit {
		t.Fatalf("capHit = false, want true")
	}
	if len(added) >= 11 {
		t.Errorf("len(added) = %d, want fewer than 11 (chain truncated by the cap)", len(added))
	}
	if len(stillChurning) == 0 {
		t.Error("stillChurning is empty, want non-empty on cap-hit")
	}
}

// sameTagMatchSet compares two []TagMatch as sets (order-independent), per pitfalls.md #2b.
func sameTagMatchSet(got, want []TagMatch) bool {
	if len(got) != len(want) {
		return false
	}
	sortedGot := append([]TagMatch(nil), got...)
	sortedWant := append([]TagMatch(nil), want...)
	byTag := func(s []TagMatch) {
		sort.Slice(s, func(i, j int) bool { return s[i].Tag < s[j].Tag })
	}
	byTag(sortedGot)
	byTag(sortedWant)
	return slices.Equal(sortedGot, sortedWant)
}

// ── Story 1.4.1: SeedTaggingRules ───────────────────────────────────────────

func TestSeedTaggingRules_should_ReturnSeedSourcedEnabledRules_When_Called(t *testing.T) {
	rules := SeedTaggingRules()

	if len(rules) < 5 {
		t.Fatalf("len(rules) = %d, want >= 5", len(rules))
	}
	for i, r := range rules {
		if r.Source != "seed" {
			t.Errorf("rules[%d].Source = %q, want %q", i, r.Source, "seed")
		}
		if !r.Enabled {
			t.Errorf("rules[%d].Enabled = false, want true", i)
		}
	}
}

func TestSeedTaggingRules_should_NotIncludeDisabledOrUserSourcedRules_When_Called(t *testing.T) {
	rules := SeedTaggingRules()

	for i, r := range rules {
		if !r.Enabled {
			t.Errorf("rules[%d] (%s) is disabled, want no disabled seed rules", i, r.ID)
		}
		if r.Source == string(SourceUser) {
			t.Errorf("rules[%d] (%s) is user-sourced, want no user-sourced seed rules", i, r.ID)
		}
	}
}

// ── Story 4.2.1: TagContentHash ─────────────────────────────────────────────

func TestTagContentHash_should_BeOrderInvariantOnTags_When_TagSetIdenticalButUnordered(t *testing.T) {
	ctx1 := SessionTaggingContext{Name: "a", Branch: "b", Path: "/p", Program: "claude", Tags: []string{"X", "Y"}}
	ctx2 := SessionTaggingContext{Name: "a", Branch: "b", Path: "/p", Program: "claude", Tags: []string{"Y", "X"}}

	if TagContentHash(ctx1) != TagContentHash(ctx2) {
		t.Errorf("TagContentHash differs for identical tag sets in different order: %q vs %q", TagContentHash(ctx1), TagContentHash(ctx2))
	}
}

func TestTagContentHash_should_ChangeHash_When_AnySingleFieldDiffers(t *testing.T) {
	base := SessionTaggingContext{Name: "a", Branch: "b", Path: "/p", Program: "claude", Tags: []string{"X", "Y"}}
	baseHash := TagContentHash(base)

	variants := []SessionTaggingContext{
		{Name: "different", Branch: base.Branch, Path: base.Path, Program: base.Program, Tags: base.Tags},
		{Name: base.Name, Branch: "different", Path: base.Path, Program: base.Program, Tags: base.Tags},
		{Name: base.Name, Branch: base.Branch, Path: "different", Program: base.Program, Tags: base.Tags},
		{Name: base.Name, Branch: base.Branch, Path: base.Path, Program: "different", Tags: base.Tags},
		{Name: base.Name, Branch: base.Branch, Path: base.Path, Program: base.Program, Tags: []string{"Z"}},
	}
	for i, v := range variants {
		if TagContentHash(v) == baseHash {
			t.Errorf("variant %d: TagContentHash unchanged (%q) despite a differing field, ctx=%+v", i, baseHash, v)
		}
	}
}
