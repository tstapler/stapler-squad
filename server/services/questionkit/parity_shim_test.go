package questionkit

import (
	"context"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/panereply"
)

// The parity scenarios call the moved code only through these names; the copy
// recorded against the base package maps them onto its unexported spellings.

const (
	sessionTestdataRoot = "../../../session/testdata"
	questionTestdata    = "testdata"
)

type deliverySinkLike = DeliverySink

func sanitizeText(s string) string { return SanitizeNotificationText(s) }

func replyResult(o sessionv1.ReplyOutcome) ReplyOutcomeResult { return ReplyOutcomeResult{Outcome: o} }

func newFlightTable(now func() time.Time) *ReplyFlightTable { return NewReplyFlightTable(now) }

func hashPayload(sess, q string, digit byte) [32]byte { return HashReplyPayload(sess, q, digit) }

func flightBegin(t *ReplyFlightTable, id string, h [32]byte) (*ReplyFlight, string) {
	f, role := t.Begin(id, ReplyPayloadHash(h))
	switch role {
	case ReplyLeader:
		return f, "leader"
	case ReplyFollower:
		return f, "follower"
	case ReplyCached:
		return f, "cached"
	case ReplyMismatch:
		return f, "mismatch"
	}
	return f, "unknown"
}

func flightFinish(t *ReplyFlightTable, id string, f *ReplyFlight, res ReplyOutcomeResult, cache bool) {
	t.Finish(id, f, res, cache)
}

func flightWait(f *ReplyFlight, ctx context.Context) (ReplyOutcomeResult, error) { return f.Wait(ctx) }

func flightSize(t *ReplyFlightTable) int { return t.Size() }

func beginTrack(ctx context.Context, rows AnswerRows, acks DeliverySink, qid string) DeliveryTrack {
	return BeginDelivery(ctx, rows, acks, qid)
}

func staleAnswerPartial() string { return StaleReasonAnswerPartial }

func applied() ActionResult { return Applied() }

func outcomeResult(o sessionv1.ActionOutcome, detail string) ActionResult { return Outcome(o, detail) }

func notWiredResult() ActionResult { return NotWired() }

func rowDialog(r *domain.GuidanceRequestData) (panereply.QuestionDialog, bool) {
	return DialogFromRow(r)
}

func rowTabbed(r *domain.GuidanceRequestData) ([]panereply.TabbedQuestion, bool) {
	return TabbedFromRow(r)
}

func tabbedHeaders(qs []panereply.TabbedQuestion) func(string) bool { return TabbedHeadersMatch(qs) }

func setMirrorFields(out *sessionv1.GuidanceRequest, d *domain.GuidanceRequestData) {
	SetGuidanceMirrorFields(out, d)
}

func judge(row *domain.GuidanceRequestData, capture string, err error) DialogVerdict {
	return JudgeRow(row, capture, err)
}

func verdictReason(v DialogVerdict) (domain.CancelReason, bool) { return v.cancelReason() }

func newMirror(store GuidanceMirrorStore, pending *PendingQuestionStore, failed, cleared func(string), enabled func() bool) *GuidanceHookMirror {
	m := NewGuidanceHookMirror(store, pending, failed, cleared, enabled)
	m.SetBound(30 * time.Second) // the 250ms production bound flakes on a loaded machine
	return m
}

func canaryMismatch() DialogMatchResult { return DialogMismatch }

func canaryMatch() DialogMatchResult { return DialogMatched }

func canaryOf(matched bool) DialogMatchResult { return DialogMatchResultOf(matched) }

func roleOf(l ItemSessionLookup, ctx context.Context, sess string) string {
	return ScheduleWakeupRoles{Lookup: l}.RoleOf(ctx, sess)
}

func newTracker(now func() time.Time) *ReopenTracker { return NewReopenTracker(now) }

func trackerPending(t *ReopenTracker) int { return len(t.pending) }

func claimWithTokenOnly() PendingQuestionClaim { return PendingQuestionClaim{token: 1} }

func claimWithStoreOnly(s *PendingQuestionStore) PendingQuestionClaim {
	return PendingQuestionClaim{store: s}
}
