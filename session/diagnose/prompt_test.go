package diagnose

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func sampleBundle() DiagnosticBundle {
	return DiagnosticBundle{
		ItemID:             "e6c2a88e",
		Description:        "the target session goes idle after every retry",
		AcceptanceCriteria: "1. [ ] session recovers without manual intervention",
		History:            "- Role: work | Commits: 3",
		PriorVerdicts:      "(no prior review verdicts)",
		SessionSnapshot:    "Path: /tmp/repo\nBranch: work-branch",
		Logs:               "(no log lines in session window)",
		Diff:               "(no uncommitted changes)",
		LinkedTranscript:   "assistant: investigating idle session",
	}
}

// TestRenderPrompt_ShouldIncludeEveryBundleSectionContent_WhenRendered is
// Task 5.1.2c's coverage: the rendered prompt contains every section's
// content, not just a subset.
func TestRenderPrompt_ShouldIncludeEveryBundleSectionContent_WhenRendered(t *testing.T) {
	bundle := sampleBundle()
	prompt := RenderPrompt(bundle)

	for _, want := range []string{
		bundle.ItemID,
		bundle.Description,
		bundle.AcceptanceCriteria,
		bundle.History,
		bundle.PriorVerdicts,
		bundle.SessionSnapshot,
		bundle.Logs,
		bundle.Diff,
		bundle.LinkedTranscript,
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("RenderPrompt output missing expected content %q", want)
		}
	}
}

// TestRenderPrompt_ShouldNameAllThreeAllowedToolCategories_WhenRendered is
// Story 5.1.2's AC: the instruction block names create_backlog_item,
// post_backlog_update, and resume_session/steer_session/write_to_session.
func TestRenderPrompt_ShouldNameAllThreeAllowedToolCategories_WhenRendered(t *testing.T) {
	prompt := RenderPrompt(sampleBundle())

	for _, tool := range []string{
		"create_backlog_item",
		"post_backlog_update",
		"resume_session",
		"steer_session",
		"write_to_session",
	} {
		if !strings.Contains(prompt, tool) {
			t.Errorf("RenderPrompt output missing allowed tool name %q", tool)
		}
	}
}

// TestRenderPrompt_ShouldNameEvidentiaryBarItem_WhenRendered is Story
// 5.1.2's AC: the bug-filing evidentiary bar cites backlog item `ce71ad1a`
// (same standard as requirements.md/plan.md).
func TestRenderPrompt_ShouldNameEvidentiaryBarItem_WhenRendered(t *testing.T) {
	prompt := RenderPrompt(sampleBundle())

	if !strings.Contains(prompt, "ce71ad1a") {
		t.Errorf("RenderPrompt output missing evidentiary bar reference to ce71ad1a, got:\n%s", prompt)
	}
}

// TestRenderPrompt_ShouldIncludeNeverBlindlyRetryInstruction_WhenRendered is
// Task 5.1.2d's coverage (adversarial-review Blocker 1): the rendered prompt
// includes the never-blindly-retry-on-write_outcome_unknown instruction,
// verbatim from server/mcp/tools_terminal.go's writeOutcomeUnknownInstruction.
func TestRenderPrompt_ShouldIncludeNeverBlindlyRetryInstruction_WhenRendered(t *testing.T) {
	prompt := RenderPrompt(sampleBundle())

	if !strings.Contains(prompt, writeOutcomeUnknownInstruction) {
		t.Error("RenderPrompt output missing the never-blindly-retry-on-write_outcome_unknown instruction verbatim")
	}
	if !strings.Contains(prompt, "write_outcome_unknown") {
		t.Error("RenderPrompt output missing the write_outcome_unknown marker text")
	}
}

// TestRenderPrompt_ShouldNameAllEightBundleSections_InEvidenceCitationRequirement
// is Task 5.1.2f's coverage (pre-mortem P1 #2): the evidence-citation
// requirement names all 8 BundleSection values the agent may cite.
func TestRenderPrompt_ShouldNameAllEightBundleSections_InEvidenceCitationRequirement(t *testing.T) {
	prompt := RenderPrompt(sampleBundle())

	if !strings.Contains(prompt, "Evidence citation requirement") {
		t.Fatal("RenderPrompt output missing the evidence citation requirement heading")
	}

	allSections := []BundleSection{
		BundleSectionDescription,
		BundleSectionAcceptanceCriteria,
		BundleSectionHistory,
		BundleSectionPriorVerdicts,
		BundleSectionSessionSnapshot,
		BundleSectionLogs,
		BundleSectionDiff,
		BundleSectionLinkedTranscript,
	}
	if len(allSections) != len(bundleSectionOrder) {
		t.Fatalf("test fixture drifted from bundleSectionOrder: got %d sections, want %d", len(allSections), len(bundleSectionOrder))
	}
	for _, section := range allSections {
		if !strings.Contains(prompt, string(section)) {
			t.Errorf("evidence citation requirement missing BundleSection value %q", section)
		}
	}
}

// TestDiagnosticBundle_Section_ShouldReturnEmptyString_WhenSectionUnrecognized
// covers the defensive default branch in DiagnosticBundle.section.
func TestDiagnosticBundle_Section_ShouldReturnEmptyString_WhenSectionUnrecognized(t *testing.T) {
	bundle := sampleBundle()
	if got := bundle.section(BundleSection("unknown")); got != "" {
		t.Errorf("expected empty string for unrecognized section, got %q", got)
	}
}

// TestAssembleDiagnosticBundle_ShouldPopulateEveryFieldFromRealItem_WhenAllDependenciesSucceed
// exercises the full-fidelity path: a live Instance, a linked transcript, and
// a verdict source that returns results.
func TestAssembleDiagnosticBundle_ShouldPopulateEveryFieldFromRealItem_WhenAllDependenciesSucceed(t *testing.T) {
	assembler := NewDiagnosticBundleAssembler(DefaultDiagnosticBundleConfig(46875))
	generator := newFakeLinkedTranscriptSummaryGenerator()

	bundle := AssembleDiagnosticBundle(context.Background(), assembler, AssembleDiagnosticBundleInput{
		ItemID: "e6c2a88e",
		Item:   BundleItem{ID: "e6c2a88e", Description: "stuck session"},
		History: []SessionHistoryEntry{
			{SessionUUID: "s1", Summary: "- Role: work | Commits: 2"},
		},
		ReviewVerdictSource: fakeVerdictSource{verdicts: []ReviewVerdictSummary{
			{OverallOutcome: "pass", Summary: "looked fine"},
		}},
		Instance:            panicOnRawFieldInstance{snap: SessionSnapshot{Path: "/repo/worktree", Branch: "work-branch"}},
		WorktreePath:        t.TempDir(), // not a git repo -- exercises the degrade-to-placeholder branch below
		TranscriptGenerator: generator,
		LinkedTranscript:    LinkedSessionTranscript{SessionID: "linked-1", SessionTitle: "linked", Content: "short transcript"},
	})

	if bundle.ItemID != "e6c2a88e" {
		t.Errorf("ItemID = %q, want e6c2a88e", bundle.ItemID)
	}
	assertBundleContains(t, "Description", bundle.Description, "stuck session")
	assertBundleContains(t, "History", bundle.History, "Role: work")
	assertBundleContains(t, "PriorVerdicts", bundle.PriorVerdicts, "looked fine")
	assertBundleContains(t, "SessionSnapshot", bundle.SessionSnapshot, "/repo/worktree")
	assertBundleContains(t, "SessionSnapshot", bundle.SessionSnapshot, "work-branch")
	// WorktreePath is a plain temp dir, not a git repo, so assembleDiff errors
	// and AssembleDiagnosticBundle degrades to a placeholder rather than
	// propagating the error.
	if bundle.Diff == "" {
		t.Error("Diff should never be empty, even on degrade")
	}
	if bundle.LinkedTranscript != "short transcript" {
		t.Errorf("LinkedTranscript = %q, want verbatim short content", bundle.LinkedTranscript)
	}
}

func assertBundleContains(t *testing.T, field, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("%s = %q, want to contain %q", field, got, want)
	}
}

// TestAssembleDiagnosticBundle_ShouldUsePlaceholders_WhenNoLiveInstanceOrLinkedSession
// covers Story 5.1.2's "no live target session" case: a stuck item whose
// session already ended still gets a diagnosable bundle.
func TestAssembleDiagnosticBundle_ShouldUsePlaceholders_WhenNoLiveInstanceOrLinkedSession(t *testing.T) {
	assembler := NewDiagnosticBundleAssembler(DefaultDiagnosticBundleConfig(46875))

	bundle := AssembleDiagnosticBundle(context.Background(), assembler, AssembleDiagnosticBundleInput{
		ItemID:              "e6c2a88e",
		Item:                BundleItem{ID: "e6c2a88e"},
		ReviewVerdictSource: fakeVerdictSource{},
	})

	if bundle.SessionSnapshot != noActiveSessionPlaceholder {
		t.Errorf("SessionSnapshot = %q, want placeholder %q", bundle.SessionSnapshot, noActiveSessionPlaceholder)
	}
	if bundle.Diff != noActiveSessionPlaceholder {
		t.Errorf("Diff = %q, want placeholder %q", bundle.Diff, noActiveSessionPlaceholder)
	}
	if bundle.LinkedTranscript != noLinkedSessionPlaceholder {
		t.Errorf("LinkedTranscript = %q, want placeholder %q", bundle.LinkedTranscript, noLinkedSessionPlaceholder)
	}
}

// TestAssembleDiagnosticBundle_ShouldDegradePriorVerdictsToPlaceholder_WhenSourceErrors
// covers the best-effort degrade path for a review-verdict query failure.
func TestAssembleDiagnosticBundle_ShouldDegradePriorVerdictsToPlaceholder_WhenSourceErrors(t *testing.T) {
	assembler := NewDiagnosticBundleAssembler(DefaultDiagnosticBundleConfig(46875))

	bundle := AssembleDiagnosticBundle(context.Background(), assembler, AssembleDiagnosticBundleInput{
		ItemID:              "e6c2a88e",
		Item:                BundleItem{ID: "e6c2a88e"},
		ReviewVerdictSource: fakeVerdictSource{err: errors.New("boom")},
	})

	if bundle.PriorVerdicts == "" {
		t.Error("PriorVerdicts should never be empty, even on degrade")
	}
	if strings.Contains(bundle.PriorVerdicts, "boom") {
		t.Errorf("PriorVerdicts leaked raw error text: %q", bundle.PriorVerdicts)
	}
}
