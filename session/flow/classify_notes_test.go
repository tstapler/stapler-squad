package flow

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyNote_should_MapKnownNotesAndCountUnclassified_When_UnknownNotePresent(t *testing.T) {
	t.Parallel()
	notes := []string{
		"dequeued: WIP slot freed",
		"dequeue spawn failed",
		"auto-reopened for PR fix (CI/review)",
		"auto-reopened after failed review verdict",
		"some new note",
	}
	want := []Cause{
		CauseDequeueClaim,
		CauseDequeueSpawnFailed,
		CausePRFixReopen,
		CauseReviewReopen,
		CauseUnclassified,
	}
	for i, n := range notes {
		assert.Equal(t, want[i], ClassifyNote(n), n)
	}

	tally := ClassifyNotes(notes)
	assert.Equal(t, 1, tally.Unclassified)
	assert.Equal(t, 1, tally.ByCause[CauseDequeueClaim])
	assert.Equal(t, 1, tally.ByCause[CausePRFixReopen])
	assert.Equal(t, 4, tally.Classified())
}

func TestClassifyNote_should_TolerateWhitespaceAndSuffix_When_NoteHasTrailingDetail(t *testing.T) {
	t.Parallel()
	assert.Equal(t, CauseDequeueClaim, ClassifyNote("  dequeued: WIP slot freed\n"))
	assert.Equal(t, CauseReviewReopen, ClassifyNote("auto-reopened after failed review verdict (attempt 2)"))
	assert.Equal(t, CauseUnclassified, ClassifyNote(""))
}

// productionNotePrefixes is the committed snapshot of note literals written by
// server/services/backlog_service_triage.go and session/transition_gate.go.
// Adding a new literal in production without a table row fails
// TestClassifierCoversKnownNotes; update both together.
var productionNotePrefixes = map[string]Cause{
	"dequeued: WIP slot freed":                  CauseDequeueClaim,
	"dequeue spawn failed":                      CauseDequeueSpawnFailed,
	"auto-reopened for PR fix (CI/review)":      CausePRFixReopen,
	"auto-reopened after failed review verdict": CauseReviewReopen,
	// Notes on non-reopen edges: intentionally unclassified, not rework causes.
	"all work sessions ended":                        CauseUnclassified,
	"request_review recovering stale/blocked review": CauseUnclassified,
}

func TestClassifierCoversKnownNotes_should_FailNamingLiteral_When_ProductionPrefixAbsentFromTable(t *testing.T) {
	t.Parallel()

	// The shipped table covers every snapshot literal.
	assert.Empty(t, notePrefixMismatches(noteRules, productionNotePrefixes))

	// Negative: a table missing one literal is reported by name.
	trimmed := make([]noteRule, 0, len(noteRules))
	for _, r := range noteRules {
		if r.prefix != "dequeue spawn failed" {
			trimmed = append(trimmed, r)
		}
	}
	got := notePrefixMismatches(trimmed, productionNotePrefixes)
	assert.Equal(t, []string{"dequeue spawn failed"}, got)
}

func TestNoteRules_should_NotShadowEachOther_When_PrefixesOverlap(t *testing.T) {
	t.Parallel()
	for i, a := range noteRules {
		for _, b := range noteRules[i+1:] {
			assert.NotEqual(t, a.prefix, b.prefix)
			if len(b.prefix) > len(a.prefix) {
				assert.False(t, strings.HasPrefix(b.prefix, a.prefix), "%q shadows %q (put longer prefix first)", a.prefix, b.prefix)
			}
		}
	}
}
