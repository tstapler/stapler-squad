package session

import (
	"strings"
	"testing"
	"time"
)

func TestRenderSessionSummaryMarkdown_should_RenderEmptyStateText_When_AllSnapshotsZero(t *testing.T) {
	md := RenderSessionSummaryMarkdown(
		"empty-session",
		"This session ended before any work was recorded.",
		true,
		DiffSnapshot{},
		DecisionsSnapshot{},
		TimelineSnapshot{StartedAt: time.Now(), StoppedAt: time.Now()},
		CostSnapshot{},
		"",
	)

	if !strings.Contains(md, "No files were changed.") {
		t.Errorf("expected empty-diff empty-state text, got:\n%s", md)
	}
	if !strings.Contains(md, "No approval requests occurred during this session.") {
		t.Errorf("expected empty-decisions empty-state text, got:\n%s", md)
	}
	if !strings.Contains(md, "No tokens were used.") {
		t.Errorf("expected empty-cost empty-state text, got:\n%s", md)
	}
	if strings.Contains(md, "0 auto-approved") {
		t.Errorf("did not expect raw zero-count rendering, got:\n%s", md)
	}
}

func TestRenderSessionSummaryMarkdown_should_ShowSubSecondDuration_When_DurationRoundsToZero(t *testing.T) {
	start := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	stop := start.Add(400 * time.Millisecond)

	md := RenderSessionSummaryMarkdown(
		"quick-session",
		"Quick session.",
		false,
		DiffSnapshot{},
		DecisionsSnapshot{},
		TimelineSnapshot{StartedAt: start, StoppedAt: stop},
		CostSnapshot{DataUnavailable: true},
		"",
	)

	if !strings.Contains(md, "Duration: <1s") {
		t.Errorf("expected 'Duration: <1s', got:\n%s", md)
	}
}

func TestRenderSessionSummaryMarkdown_should_RenderAllSectionsAndCorrectPercentages_When_Populated(t *testing.T) {
	start := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	stop := start.Add(10 * time.Minute)

	diff := DiffSnapshot{FilesChanged: 3, Added: 42, Removed: 7}
	decisions := DecisionsSnapshot{AutoApproved: 5, ManuallyApproved: 3, Denied: 0, ReviewQueueResolved: 0, StillOpen: 0}
	timeline := TimelineSnapshot{StartedAt: start, StoppedAt: stop}
	cost := CostSnapshot{TotalTokens: 128000, EstimatedCostUSD: 1.92}

	md := RenderSessionSummaryMarkdown("my-session", "Did some work.", false, diff, decisions, timeline, cost, "https://example.com/diff")

	for _, heading := range []string{"## What Was Done", "## Changes", "## Decisions", "## Timeline", "## Token Usage"} {
		if !strings.Contains(md, heading) {
			t.Errorf("expected heading %q in markdown, got:\n%s", heading, md)
		}
	}
	if !strings.Contains(md, "[View full diff](https://example.com/diff)") {
		t.Errorf("expected diff link, got:\n%s", md)
	}
	// 5 of 8 total -> 62.5%
	if !strings.Contains(md, "62.5%") {
		t.Errorf("expected 62.5%% for 5/8 auto-approved, got:\n%s", md)
	}
	if !strings.Contains(md, "$1.92") {
		t.Errorf("expected cost $1.92, got:\n%s", md)
	}
}
