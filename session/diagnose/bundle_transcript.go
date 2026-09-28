package diagnose

import (
	"context"
	"time"
)

// linkedTranscriptTooLargeNote is the one-line fallback substituted into the
// LinkedTranscript section when the transcript is over budget and summary
// generation itself fails or times out (Story 2.2.1 AC) -- assembly must
// never block on it.
const linkedTranscriptTooLargeNote = "Linked session transcript too large to summarize; summary generation failed."

// Mirror of session.HandoffSummaryStatus's string values. Package diagnose
// cannot import package session to reuse that type directly: session already
// transitively imports package diagnose (session -> session/tmux ->
// session/diagnose), so the reverse import would close an import cycle.
const (
	handoffSummaryRowStatusReady = "ready"
	handoffSummaryRowStatusError = "error"
)

// handoffSummaryTimeout bounds how long assembleLinkedTranscript polls
// FindRowBySessionID for a generation to finish. Mirrors
// session.handoffSummaryTimeout's value (that var is unexported in package
// session and can't be imported -- see the import-cycle note above). A var,
// not a const, so tests can lower it to exercise the timeout path without
// waiting the full duration.
var handoffSummaryTimeout = 60 * time.Second

// handoffSummaryPollInterval is the delay between FindRowBySessionID polls.
// A var for the same reason as handoffSummaryTimeout.
var handoffSummaryPollInterval = 500 * time.Millisecond

// HandoffSummaryRow is the minimal view of a handoff-summary row
// assembleLinkedTranscript needs: just enough to tell whether generation
// finished and, if so, with what text. Deliberately not session.HandoffSummary
// itself (see the import-cycle note above) -- whatever later step wires a
// real *session.HandoffSummaryGenerator into LinkedTranscriptSummaryGenerator
// adapts session.HandoffSummary into this shape.
type HandoffSummaryRow struct {
	Status      string
	SummaryText string
}

// LinkedTranscriptSummaryGenerator is the subset of
// session.HandoffSummaryGenerator's API assembleLinkedTranscript needs to
// compact an over-budget linked-session transcript. Defined locally instead
// of depending on *session.HandoffSummaryGenerator directly because of the
// import-cycle constraint described on HandoffSummaryRow.
//
// Method shapes mirror session.HandoffSummaryGenerator's BeginGeneration,
// GenerateAndPersist, and FindRowBySessionID exactly, modulo the
// HandoffSummaryRow substitution above.
type LinkedTranscriptSummaryGenerator interface {
	BeginGeneration(ctx context.Context, sourceSessionID, sourceSessionTitle string) (release func(), startedAt time.Time, started bool, err error)
	GenerateAndPersist(ctx context.Context, sourceSessionID, sourceSessionTitle string, release func(), now time.Time)
	FindRowBySessionID(ctx context.Context, sessionID string) (*HandoffSummaryRow, error)
}

// LinkedSessionTranscript identifies the linked session whose transcript is
// being assembled into the LinkedTranscript bundle section, bundling the
// fields BeginGeneration/GenerateAndPersist/FindRowBySessionID all key on
// (SessionID) plus the two pieces of content those calls need
// (SessionTitle, Content) into one value instead of a bare parameter pile.
type LinkedSessionTranscript struct {
	SessionID    string
	SessionTitle string
	Content      string
}

// assembleLinkedTranscript returns the content for the LinkedTranscript
// bundle section: linked.Content verbatim when it fits budget.MaxBytes, or --
// when it doesn't -- a summary compacted via generator's
// BeginGeneration/GenerateAndPersist pipeline, polling FindRowBySessionID
// (bounded by handoffSummaryTimeout) until the row resolves to ready or
// error. On error, on a BeginGeneration write failure, or on timeout, it
// returns linkedTranscriptTooLargeNote rather than blocking or failing the
// rest of bundle assembly.
func assembleLinkedTranscript(ctx context.Context, generator LinkedTranscriptSummaryGenerator, linked LinkedSessionTranscript, budget SectionBudget) string {
	if len(linked.Content) <= budget.MaxBytes {
		return linked.Content
	}

	release, startedAt, started, err := generator.BeginGeneration(ctx, linked.SessionID, linked.SessionTitle)
	if err != nil {
		return linkedTranscriptTooLargeNote
	}
	if started {
		// GenerateAndPersist is meant to run as a detached goroutine (see its
		// doc comment on session.HandoffSummaryGenerator) so it can keep
		// running past this call's context -- use context.Background(), not
		// ctx, so canceling/timing out the poll below doesn't also cancel a
		// still-in-flight generation another caller may be waiting on.
		go generator.GenerateAndPersist(context.Background(), linked.SessionID, linked.SessionTitle, release, startedAt)
	}
	// If !started, a generation for this session is already in flight
	// elsewhere (BeginGeneration's dedup guard) -- fall through and poll for
	// its result rather than starting a duplicate.

	return pollForLinkedTranscriptSummary(ctx, generator, linked.SessionID)
}

// pollForLinkedTranscriptSummary polls FindRowBySessionID, bounded by
// handoffSummaryTimeout, until the row is ready (returns its SummaryText),
// error (returns the fallback note), or the timeout elapses (also the
// fallback note).
func pollForLinkedTranscriptSummary(ctx context.Context, generator LinkedTranscriptSummaryGenerator, sessionID string) string {
	pollCtx, cancel := context.WithTimeout(ctx, handoffSummaryTimeout)
	defer cancel()

	ticker := time.NewTicker(handoffSummaryPollInterval)
	defer ticker.Stop()

	for {
		if row, err := generator.FindRowBySessionID(pollCtx, sessionID); err == nil && row != nil {
			switch row.Status {
			case handoffSummaryRowStatusReady:
				return row.SummaryText
			case handoffSummaryRowStatusError:
				return linkedTranscriptTooLargeNote
			}
		}

		select {
		case <-pollCtx.Done():
			return linkedTranscriptTooLargeNote
		case <-ticker.C:
		}
	}
}
