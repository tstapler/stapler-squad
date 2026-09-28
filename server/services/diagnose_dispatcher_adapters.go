package services

// diagnose_dispatcher_adapters.go — the real-session-package -> session/diagnose
// DTO adapters DiagnoseDispatcher.assembleBundle depends on, split out of
// diagnose_dispatcher.go purely to keep that orchestration file's own length
// manageable. See that file's package doc comment for the import-cycle
// rationale these adapters exist to bridge.

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/diagnose"
)

// diagnosticDataSourceVerdictAdapter adapts DiagnosticDataSource's
// GetRecentReviewVerdictSummaries (real session.ReviewVerdictSummary) into
// diagnose.ReviewVerdictSource's local ReviewVerdictSummary DTO.
type diagnosticDataSourceVerdictAdapter struct {
	source DiagnosticDataSource
}

func (a diagnosticDataSourceVerdictAdapter) GetRecentReviewVerdictSummaries(ctx context.Context, itemID string, limit int) ([]diagnose.ReviewVerdictSummary, error) {
	verdicts, err := a.source.GetRecentReviewVerdictSummaries(ctx, itemID, limit)
	if err != nil {
		return nil, fmt.Errorf("diagnostic data source verdict adapter: item %s: %w", itemID, err)
	}
	out := make([]diagnose.ReviewVerdictSummary, len(verdicts))
	for i, v := range verdicts {
		out[i] = diagnose.ReviewVerdictSummary{
			OverallOutcome: v.OverallOutcome,
			Summary:        v.Summary,
			CreatedAt:      v.CreatedAt,
		}
	}
	return out, nil
}

// instanceSnapshotAdapter adapts a real *session.Instance to satisfy
// diagnose.InstanceSnapshotter, reading exclusively through inst.Snapshot()
// (never a raw i.Path/i.Branch field read) per
// .claude/rules/instance-lock-free-reads.md.
type instanceSnapshotAdapter struct {
	inst *session.Instance
}

func (a instanceSnapshotAdapter) Snapshot() diagnose.SessionSnapshot {
	snap := a.inst.Snapshot()
	return diagnose.SessionSnapshot{Path: snap.Path, Branch: snap.Branch}
}

// handoffSummaryGeneratorAdapter adapts a real *session.HandoffSummaryGenerator
// to satisfy diagnose.LinkedTranscriptSummaryGenerator, converting
// *session.HandoffSummary -> *diagnose.HandoffSummaryRow at this boundary
// (session/diagnose/bundle_transcript.go's import-cycle constraint: that
// package can't depend on *session.HandoffSummaryGenerator or
// session.HandoffSummary directly). BeginGeneration/GenerateAndPersist's
// signatures already match LinkedTranscriptSummaryGenerator's exactly (see
// that interface's doc comment), so only FindRowBySessionID's return type
// needs adapting.
type handoffSummaryGeneratorAdapter struct {
	generator *session.HandoffSummaryGenerator
}

func (a handoffSummaryGeneratorAdapter) BeginGeneration(ctx context.Context, sourceSessionID, sourceSessionTitle string) (func(), time.Time, bool, error) {
	return a.generator.BeginGeneration(ctx, sourceSessionID, sourceSessionTitle)
}

func (a handoffSummaryGeneratorAdapter) GenerateAndPersist(ctx context.Context, sourceSessionID, sourceSessionTitle string, release func(), now time.Time) {
	a.generator.GenerateAndPersist(ctx, sourceSessionID, sourceSessionTitle, release, now)
}

func (a handoffSummaryGeneratorAdapter) FindRowBySessionID(ctx context.Context, sessionID string) (*diagnose.HandoffSummaryRow, error) {
	row, err := a.generator.FindRowBySessionID(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("handoff summary generator adapter: find row for session %s: %w", sessionID, err)
	}
	if row == nil {
		//nolint:nilnil // mirrors diagnose.LinkedTranscriptSummaryGenerator's documented not-found contract (nil, nil), see session/diagnose/bundle_transcript.go's FindRowBySessionID doc comment.
		return nil, nil
	}
	return &diagnose.HandoffSummaryRow{Status: row.Status, SummaryText: row.SummaryText}, nil
}

// NewHandoffSummaryTranscriptGenerator wraps generator to satisfy
// diagnose.LinkedTranscriptSummaryGenerator, for wiring a real
// *session.HandoffSummaryGenerator into DiagnoseDispatcherDeps.Transcripts.
func NewHandoffSummaryTranscriptGenerator(generator *session.HandoffSummaryGenerator) diagnose.LinkedTranscriptSummaryGenerator {
	return handoffSummaryGeneratorAdapter{generator: generator}
}
