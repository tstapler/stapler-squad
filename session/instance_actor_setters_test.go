package session

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/classifier"
)

func TestSetAutoApprove_should_NotRestart_When_StatusPaused(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Status = Paused

	persisted := false
	err := inst.SetAutoApprove(true, func() error {
		persisted = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, inst.AutoApprove)
	assert.True(t, persisted, "persist must run even when no restart occurs")
}

func TestSetAutoApprove_should_ReturnErrorAndSkipRestart_When_PersistFails(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Status = Active
	inst.started.Store(true)
	persistErr := errors.New("disk full")

	err := inst.SetAutoApprove(true, func() error {
		return persistErr
	})

	require.ErrorIs(t, err, persistErr)
	// Matches the documented ordering (set -> persist -> restart): the in-memory
	// field is already flipped by the time persist runs, but a persist failure
	// must short-circuit before any restart is attempted.
	assert.True(t, inst.AutoApprove)
}

// TestSetAutoApprove_RestartError_PersistAlreadyRan mirrors
// TestSwitchProgram_RestartError_PersistAlreadyRan (session/instance_program_test.go):
// a restart failure must not roll back or hide the already-persisted field change, so
// the caller (UpdateSession) and the persisted store agree on the new value even though
// the session didn't actually restart.
func TestSetAutoApprove_RestartError_PersistAlreadyRan(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Status = Active
	inst.started.Store(true)
	inst.Path = "" // forces Restart() to fail deterministically, no real tmux involved

	persistedValue := false
	persistRan := false
	err := inst.SetAutoApprove(true, func() error {
		persistRan = true
		persistedValue = inst.AutoApprove
		return nil
	})

	require.Error(t, err, "Restart should fail with no working directory configured")
	assert.True(t, persistRan, "persist callback must run before the restart attempt")
	assert.True(t, persistedValue, "persist callback must observe the new value, i.e. run after the field is set")
	assert.True(t, inst.AutoApprove, "AutoApprove field must reflect the change even though restart failed")
}

// TestSetAutoApprove_SerializesWithSwitchProgram_When_ConcurrentCalls is the AC7
// regression guard: SetAutoApprove and SwitchProgram must serialize through the shared
// restartTriggerMu so a racing program-switch and auto-approve toggle can't both observe
// Status == Active and double-restart the same tmux session. Proven deterministically (not
// just via -race) by blocking SetAutoApprove mid-persist and asserting a concurrent
// SwitchProgram call cannot proceed until the lock is released.
func TestSetAutoApprove_SerializesWithSwitchProgram_When_ConcurrentCalls(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Program = "claude"
	inst.Status = Active
	inst.started.Store(true)
	inst.Path = "" // forces Restart() to fail deterministically, no real tmux involved

	entered := make(chan struct{})
	release := make(chan struct{})

	go func() {
		_ = inst.SetAutoApprove(true, func() error {
			close(entered)
			<-release
			return nil
		})
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("SetAutoApprove never reached persist callback")
	}

	done := make(chan struct{})
	go func() {
		_, _, _ = inst.SwitchProgram(context.Background(), "aider", nil)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("SwitchProgram completed while SetAutoApprove held restartTriggerMu — the two setters are not sharing a lock")
	case <-time.After(100 * time.Millisecond):
		// Expected: SwitchProgram is blocked waiting on restartTriggerMu.
	}

	close(release)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SwitchProgram never completed after SetAutoApprove released restartTriggerMu")
	}
}

// --- Story 3.3.1: reclassifyTagsLocked fixpoint hook ---

// bugfixSeedRule is a minimal TaggingRule mirroring SeedTaggingRules()' seed-tag-bugfix entry,
// used directly (rather than the full seed set) so these tests assert against one predictable
// rule instead of every seed rule's incidental side effects.
func bugfixSeedRule() classifier.TaggingRule {
	return classifier.TaggingRule{
		RuleMeta:      classifier.RuleMeta{ID: "seed-bugfix", Name: "Bugfix branch", Priority: 100, Enabled: true, Source: "seed"},
		BranchPattern: regexp.MustCompile(`^(bugfix|fix)/`),
		OutputTag:     "Bugfix",
	}
}

func TestReclassifyTagsLocked_should_ApplyMatchingSeedRuleTagSynchronously_When_BranchSetterCalled(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Program = "claude"
	inst.Branch = "main"
	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules([]classifier.TaggingRule{bugfixSeedRule()})
	inst.SetTaggingEngine(engine)

	inst.SetGitHubResolution(GitHubResolution{Path: inst.Path, Branch: "bugfix/pr-poller"})

	assert.Contains(t, inst.GetTags(), "Bugfix")
	assert.Equal(t, "seed-bugfix", inst.RuleTagProvenance["Bugfix"])
}

func TestReclassifyTagsLocked_should_RetractRuleOwnedTag_When_ConditionNoLongerHolds(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Branch = "bugfix/x"
	inst.Tags = []string{"Important"}
	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules([]classifier.TaggingRule{bugfixSeedRule()})
	inst.SetTaggingEngine(engine)

	// Prime: apply the rule via a mutating setter (SetGitHubResolution with an unchanged
	// branch — Branch stays "bugfix/x", Path is set for the first time).
	inst.SetGitHubResolution(GitHubResolution{Path: inst.Path, Branch: "bugfix/x"})
	require.Contains(t, inst.GetTags(), "Bugfix")

	// Rename off the matching branch.
	inst.SetGitHubResolution(GitHubResolution{Path: inst.Path, Branch: "main"})

	tags := inst.GetTags()
	assert.NotContains(t, tags, "Bugfix", "rule-owned tag must be retracted once its condition no longer holds")
	assert.Contains(t, tags, "Important", "an unrelated user tag must never be touched by retraction")
}

func TestReclassifyTagsLocked_should_NotReAddSuppressedTag_When_RuleConditionMatchesAgain(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Branch = "bugfix/x"
	inst.SuppressedRuleTags = map[string]bool{"Bugfix": true}
	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules([]classifier.TaggingRule{bugfixSeedRule()})
	inst.SetTaggingEngine(engine)

	inst.SetGitHubResolution(GitHubResolution{Path: inst.Path, Branch: "bugfix/x"})

	assert.NotContains(t, inst.GetTags(), "Bugfix", "a suppressed tag must never be re-added by the fixpoint")
}

func TestReclassifyTagsLocked_should_PreserveLLMProvenancedTag_When_UnrelatedSetterRuns(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Tags = []string{"Feature"}
	inst.RuleTagProvenance = map[string]string{"Feature": llmSentinelRuleID}
	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules(nil) // no sync rule can match "Feature" -- and none has ID "llm"
	inst.SetTaggingEngine(engine)

	inst.SetProgram("aider")

	assert.Contains(t, inst.GetTags(), "Feature", "an LLM-provenanced tag must survive the sync retraction loop")
	assert.Equal(t, llmSentinelRuleID, inst.RuleTagProvenance["Feature"])
}

func TestReclassifyTagsLocked_should_RetractTag_When_OwningRuleDeletedViaCRUD(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Branch = "bugfix/x"
	inst.Tags = []string{"Bugfix"}
	inst.RuleTagProvenance = map[string]string{"Bugfix": "seed-bugfix"}
	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules([]classifier.TaggingRule{bugfixSeedRule()})
	inst.SetTaggingEngine(engine)

	// Simulate a CRUD DeleteTaggingRule: the rule with ID "seed-bugfix" no longer exists.
	engine.ReplaceRules(nil)

	inst.SetProgram("aider")

	assert.NotContains(t, inst.GetTags(), "Bugfix", "a tag whose owning rule was deleted must be retracted, not orphaned")
	_, hasProvenance := inst.RuleTagProvenance["Bugfix"]
	assert.False(t, hasProvenance)
}

// TestAllTagRelevantSetters_should_CallReclassifyTagsLocked_When_SourceScanned re-runs Task
// 3.3.1d's grep/AST enumeration of every xxxLocked setter that mutates
// Branch/Path/Program/Title in instance_actor_setters.go, and asserts each one's function
// body calls reclassifyTagsLocked — a structural safeguard against a future setter being
// wired incorrectly (architecture-review.md Blocker 3). Deliberately scoped to
// instance_actor_setters.go only; instance_worktree.go's unlocked setupFirstTimeWorktree
// path is covered separately by Story 3.3.2's ReclassifyTagsAfterCreate tests.
func TestAllTagRelevantSetters_should_CallReclassifyTagsLocked_When_SourceScanned(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "instance_actor_setters.go", nil, 0)
	require.NoError(t, err, "expected to parse instance_actor_setters.go")

	tagRelevantFields := map[string]bool{"Branch": true, "Path": true, "Program": true, "Title": true}

	checked := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !strings.HasSuffix(fn.Name.Name, "Locked") {
			continue
		}
		if !mutatesTagRelevantFieldOnInst(fn.Body, tagRelevantFields) {
			continue
		}
		checked++
		assert.True(t, callsReclassifyTagsLocked(fn.Body),
			"%s mutates a tag-relevant field (Branch/Path/Program/Title) but never calls reclassifyTagsLocked", fn.Name.Name)
	}
	assert.GreaterOrEqual(t, checked, 3, "expected at least the 3 setters enumerated by Task 3.3.1d (setProgramLocked, setTitleDirectLocked, setGitHubResolutionLocked)")
}

// mutatesTagRelevantFieldOnInst reports whether body contains an assignment to
// s.inst.<Field> for one of fields — an AST-based replacement for the former
// `s\.inst\.(Branch|Path|Program|Title)\s*=` regex, which broke on reformatting.
func mutatesTagRelevantFieldOnInst(body ast.Node, fields map[string]bool) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			sel, ok := lhs.(*ast.SelectorExpr)
			if !ok || !fields[sel.Sel.Name] {
				continue
			}
			inst, ok := sel.X.(*ast.SelectorExpr)
			if !ok || inst.Sel.Name != "inst" {
				continue
			}
			if recv, ok := inst.X.(*ast.Ident); ok && recv.Name == "s" {
				found = true
			}
		}
		return true
	})
	return found
}

// callsReclassifyTagsLocked reports whether body contains a call to reclassifyTagsLocked
// anywhere in its statement tree (including inside nested blocks/closures).
func callsReclassifyTagsLocked(body ast.Node) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "reclassifyTagsLocked" {
			found = true
		}
		return true
	})
	return found
}

// --- Story 3.4.1: Unclassified coexistence rule ---

func TestDropUnclassifiedIfOtherTagsPresentLocked_should_RemoveUnclassified_When_RealTagAdded(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Branch = "bugfix/x"
	inst.Tags = []string{UnclassifiedTag}
	inst.RuleTagProvenance = map[string]string{UnclassifiedTag: llmSentinelRuleID}
	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules([]classifier.TaggingRule{bugfixSeedRule()})
	inst.SetTaggingEngine(engine)

	inst.SetProgram("aider")

	tags := inst.GetTags()
	assert.Contains(t, tags, "Bugfix")
	assert.NotContains(t, tags, UnclassifiedTag)
}

func TestDropUnclassifiedIfOtherTagsPresentLocked_should_LeaveUnclassified_When_NoOtherTagPresent(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Tags = []string{UnclassifiedTag}
	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules(nil)
	inst.SetTaggingEngine(engine)

	inst.SetProgram("aider")

	assert.Equal(t, []string{UnclassifiedTag}, inst.GetTags())
}

// --- Story 2.3.3 addendum: fire-count recording from reclassifyTagsLocked ---

// fakeTagFireRecorder records every RecordTaggingRuleFire call for assertion, satisfying
// session.TagFireRecorder structurally (no import needed the other way — see that
// interface's doc comment).
type fakeTagFireRecorder struct {
	fired []string
}

func (f *fakeTagFireRecorder) RecordTaggingRuleFire(ruleID string) {
	f.fired = append(f.fired, ruleID)
}

func TestReclassifyTagsLocked_should_RecordTagFire_When_RuleMatchesRegardlessOfSuppression(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Branch = "bugfix/x"
	inst.SuppressedRuleTags = map[string]bool{"Bugfix": true}
	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules([]classifier.TaggingRule{bugfixSeedRule()})
	inst.SetTaggingEngine(engine)
	recorder := &fakeTagFireRecorder{}
	inst.SetTagFireRecorder(recorder)

	inst.SetProgram("aider")

	assert.Equal(t, []string{"seed-bugfix"}, recorder.fired, "the rule fired (and must be recorded) even though \"Bugfix\" is suppressed and never actually applied")
	assert.NotContains(t, inst.GetTags(), "Bugfix", "sanity check: the tag itself must indeed be suppressed")
}
