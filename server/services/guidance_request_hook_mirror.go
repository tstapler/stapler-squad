package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// guidance_request_hook_mirror.go mirrors AskUserQuestion hooks into durable
// GuidanceRequest rows (ADR-003). The mirror is best effort: it never delays
// or changes the hook's reply, and PendingQuestionStore stays the in-memory
// verified-proof cache for the keystroke path.

// guidanceMirrorBound caps how long the hook path waits for the mirror write.
const guidanceMirrorBound = 250 * time.Millisecond

// Failure reasons of guidance_mirror_failed_total.
const (
	mirrorFailTimeout    = "timeout"
	mirrorFailError      = "error"
	mirrorFailCapped     = "cap_exceeded"
	mirrorFailNoPayload  = "no_payload"
	mirrorFailBadPayload = "invalid_payload"
)

type guidanceMirrorStore interface {
	CreateMirroredGuidanceRequest(ctx context.Context, req domain.GuidanceRequest, capLimit int) (*session.GuidanceRequestData, error)
	CancelGuidanceRequest(ctx context.Context, id uuid.UUID, reason string) (bool, error)
	GetGuidanceRequest(ctx context.Context, id uuid.UUID) (*session.GuidanceRequestData, error)
	ListAllPendingGuidanceRequests(ctx context.Context) ([]*session.GuidanceRequestData, error)
	ListOpenGuidanceRequestsByAsker(ctx context.Context, askerSessionUUID string, sources ...domain.QuestionSource) ([]*session.GuidanceRequestData, error)
	PurgeTerminalGuidanceQuestions(ctx context.Context, cutoff time.Time) (int, error)
}

// GuidanceHookMirror writes, clears and re-hydrates hook-source guidance rows.
type GuidanceHookMirror struct {
	store   guidanceMirrorStore
	pending *PendingQuestionStore
	bound   time.Duration
	failed  func(reason string)
	cleared func(reason string)
	// capture takes one fresh pane capture; nil disables on-demand re-verification.
	capture func(ctx context.Context, sessionUUID string) (string, error)
	// retireCard moves a cleared question's card out of Needs you; nil leaves it.
	retireCard func(rowID string)
	// enabled is read on every Mirror call so a live flag flip applies without a restart.
	enabled func() bool
}

// SetCardRetirer installs what moves the card of a question cleared by a terminal
// answer or a vanished dialog out of Needs you (UX-AC26: it flips, then settles).
func (m *GuidanceHookMirror) SetCardRetirer(fn func(rowID string)) { m.retireCard = fn }

// SetPaneCapture installs the capture used to re-verify open rows at the cap.
func (m *GuidanceHookMirror) SetPaneCapture(fn func(ctx context.Context, sessionUUID string) (string, error)) {
	m.capture = fn
}

// NewGuidanceHookMirror builds the mirror. failed and cleared count
// guidance_mirror_failed_total and guidance_mirror_cleared_total; either may be nil.
func NewGuidanceHookMirror(store guidanceMirrorStore, pending *PendingQuestionStore, failed, cleared func(reason string)) *GuidanceHookMirror {
	return &GuidanceHookMirror{store: store, pending: pending, bound: guidanceMirrorBound, failed: failed, cleared: cleared,
		enabled: func() bool { return effectiveFlag(askUserQuestionStructuredFlagName, scopeStageRender) }}
}

// HookMirrorRequest is one AskUserQuestion to mirror.
type HookMirrorRequest struct {
	SessionUUID string
	// HookCallID is the hook's tool_use_id; empty falls back to the payload hash.
	HookCallID string
	Asked      AskedQuestion
	// Replyable is true when the sender proof verified and the shape is single-select
	// or the measured tabbed multi-question dialog (AskedQuestion.Tabbed).
	Replyable bool
}

// route is live_keystroke for a verified single-select. A view-only row stays
// unrouted: ValidateDelivery admits read_only only with a stale, saved_for_later
// or failed state, none of which describes an open, never-attempted question.
func (r HookMirrorRequest) route() domain.DeliveryRoute {
	if r.Replyable {
		return domain.DeliveryRouteLiveKeystroke
	}
	return ""
}

// Mirror writes the row within the bound and returns its id. Any failure is
// counted and logged and yields ok=false; the caller carries on without a row.
func (m *GuidanceHookMirror) Mirror(req HookMirrorRequest) (id string, ok bool) {
	// Flag off writes no durable row, so the hook path stays exactly the legacy one (ADR-008).
	if m.enabled != nil && !m.enabled() {
		return "", false
	}
	payload, err := redactedPayload(req.Asked.Payload)
	if err != nil {
		m.fail(mirrorFailBadPayload, req.SessionUUID, err)
		return "", false
	}
	if payload.IsZero() {
		m.fail(mirrorFailNoPayload, req.SessionUUID, nil)
		return "", false
	}
	gr, err := domain.NewGuidanceRequest(domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: req.SessionUUID, Source: domain.QuestionSourceHook,
		AskerSessionUUID: req.SessionUUID, Payload: payload, HookCallID: domain.HookCallID(req.HookCallID),
		DeliveryRoute: req.route(),
	})
	if err != nil {
		m.fail(mirrorFailBadPayload, req.SessionUUID, err)
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), m.bound)
	defer cancel()
	type result struct {
		row *session.GuidanceRequestData
		err error
	}
	done := make(chan result, 1) // buffered so a late write never blocks its goroutine
	go func() {
		row, cerr := m.store.CreateMirroredGuidanceRequest(ctx, gr, 0)
		if errors.Is(cerr, session.ErrPendingCapExceeded) && m.reverifyOpen(ctx, req.SessionUUID) > 0 {
			row, cerr = m.store.CreateMirroredGuidanceRequest(ctx, gr, 0)
		}
		done <- result{row, cerr}
	}()
	select {
	case res := <-done:
		switch {
		case errors.Is(res.err, session.ErrPendingCapExceeded):
			m.fail(mirrorFailCapped, req.SessionUUID, res.err)
		case res.err != nil:
			m.fail(mirrorFailError, req.SessionUUID, res.err)
		default:
			return res.row.ID, true
		}
	case <-ctx.Done():
		m.fail(mirrorFailTimeout, req.SessionUUID, ctx.Err())
	}
	return "", false
}

func (m *GuidanceHookMirror) fail(reason, sessionUUID string, err error) {
	if m.failed != nil {
		m.failed(reason)
	}
	log.Warn("[GuidanceMirror] hook question not mirrored", "reason", reason, "session", sessionUUID, "err", err)
}

// redactedPayload re-validates p with every text field scanned for secrets.
func redactedPayload(p domain.QuestionPayload) (domain.QuestionPayload, error) {
	if p.IsZero() {
		return p, nil
	}
	qs := p.Questions()
	for i := range qs {
		qs[i].Text, _ = RedactSecrets(qs[i].Text)
		qs[i].Header, _ = RedactSecrets(qs[i].Header)
		for j := range qs[i].Options {
			qs[i].Options[j].Label, _ = RedactSecrets(qs[i].Options[j].Label)
			qs[i].Options[j].Description, _ = RedactSecrets(qs[i].Options[j].Description)
		}
	}
	return domain.NewQuestionPayload(qs)
}

// dialogFromRow rebuilds the on-screen dialog of a replyable hook row: one
// single-select question that passes the same label rules as a live parse.
func dialogFromRow(row *session.GuidanceRequestData) (session.QuestionDialog, bool) {
	if row.Source != domain.QuestionSourceHook || row.DeliveryRoute != domain.DeliveryRouteLiveKeystroke {
		return session.QuestionDialog{}, false
	}
	payload, err := domain.ParseQuestionPayload([]byte(row.QuestionsJSON))
	if err != nil {
		return session.QuestionDialog{}, false
	}
	qs := payload.Questions()
	if len(qs) != 1 || qs[0].MultiSelect || qs[0].Header == "" {
		return session.QuestionDialog{}, false
	}
	labels := make([]string, len(qs[0].Options))
	for i, o := range qs[0].Options {
		labels[i] = o.Label
	}
	if !replyableLabels(labels) {
		return session.QuestionDialog{}, false
	}
	return session.QuestionDialog{Question: qs[0].Text, Header: qs[0].Header, Labels: labels}, true
}

// tabbedFromRow returns the questions of a replyable hook row in the measured
// tabbed shape. It is the identity the router and the descriptor builder share:
// such a row is answered by the stepwise driver and re-verified with
// session.ParseTabbedFrame, never session.DialogMatch.
func tabbedFromRow(row *session.GuidanceRequestData) ([]session.TabbedQuestion, bool) {
	if row.Source != domain.QuestionSourceHook || row.DeliveryRoute != domain.DeliveryRouteLiveKeystroke || row.QuestionsJSON == "" {
		return nil, false
	}
	payload, err := domain.ParseQuestionPayload([]byte(row.QuestionsJSON))
	if err != nil {
		return nil, false
	}
	qs := tabbedQuestions(payload)
	return qs, tabbedReplyable(payload)
}

// tabbedHeadersMatch reports whether a capture shows a recognised tabbed frame
// whose tab headers are the row's.
func tabbedHeadersMatch(qs []session.TabbedQuestion) func(capture string) bool {
	return func(capture string) bool {
		f, ok := session.ParseTabbedFrame(capture)
		if !ok || len(f.Tabs) != len(qs) {
			return false
		}
		for i, t := range f.Tabs {
			if t.Header != strings.Join(strings.Fields(qs[i].Header), " ") {
				return false
			}
		}
		return true
	}
}

// register puts a durable row back in the verified-proof cache. The row id is
// the question id, so a card rendered before a restart still addresses it.
func (m *GuidanceHookMirror) register(row *session.GuidanceRequestData) bool {
	dialog, ok := dialogFromRow(row)
	if !ok || row.AskerSessionUUID == "" {
		return false
	}
	m.pending.Register(PendingQuestion{
		QuestionID: row.ID, SessionUUID: row.AskerSessionUUID, Dialog: dialog, Token: QuestionToken(dialog.Question),
	})
	return true
}

// Rehydrate re-registers every open replyable hook row after a restart and
// returns how many it registered. It writes nothing to a pane: the claim path
// re-verifies session.DialogMatch before any byte, so a dialog that changed
// while the service was down fails closed as STALE.
func (m *GuidanceHookMirror) Rehydrate(ctx context.Context) (int, error) {
	rows, err := m.store.ListAllPendingGuidanceRequests(ctx)
	if err != nil {
		return 0, fmt.Errorf("rehydrate pending questions: %w", err)
	}
	n := 0
	for _, row := range rows {
		if m.register(row) {
			n++
		}
	}
	return n, nil
}

// EnsureRegistered re-registers one open hook row whose cache entry expired
// (the cache TTL is shorter than a row may stay answerable). It reports
// whether the key is now registered.
func (m *GuidanceHookMirror) EnsureRegistered(ctx context.Context, key QuestionKey) bool {
	if _, ok := m.pending.Peek(key); ok {
		return true
	}
	id, err := uuid.Parse(key.QuestionID)
	if err != nil {
		return false
	}
	row, err := m.store.GetGuidanceRequest(ctx, id)
	if err != nil || row.Status() != session.GuidanceRequestStatusPending || row.AskerSessionUUID != key.SessionUUID {
		return false
	}
	return m.register(row)
}

// Clear cancels one mirrored row because its dialog was resolved elsewhere and
// drops its cache entry, so terminal-answered questions never hold a cap slot.
func (m *GuidanceHookMirror) Clear(ctx context.Context, rowID string, reason domain.CancelReason) error {
	id, err := uuid.Parse(rowID)
	if err != nil {
		return fmt.Errorf("clear mirror: %w", err)
	}
	row, err := m.store.GetGuidanceRequest(ctx, id)
	if err != nil {
		return fmt.Errorf("clear mirror %s: %w", rowID, err)
	}
	applied, err := m.store.CancelGuidanceRequest(ctx, id, reason.String())
	if err != nil {
		return err
	}
	m.pending.Forget(QuestionKey{SessionUUID: row.AskerSessionUUID, QuestionID: rowID})
	if applied && m.cleared != nil {
		m.cleared(reason.String())
	}
	if applied && m.retireCard != nil && reason != domain.CancelReasonSessionEnded {
		m.retireCard(rowID)
	}
	return nil
}

// ClearByHookCall clears the open hook row of one tool call (a PostToolUse receipt).
func (m *GuidanceHookMirror) ClearByHookCall(ctx context.Context, sessionUUID, hookCallID string, reason domain.CancelReason) error {
	if hookCallID == "" {
		return nil
	}
	rows, err := m.store.ListOpenGuidanceRequestsByAsker(ctx, sessionUUID, domain.QuestionSourceHook)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if string(row.HookCallID) == hookCallID {
			if cerr := m.Clear(ctx, row.ID, reason); cerr != nil {
				return cerr
			}
		}
	}
	return nil
}

// reverifyOpen takes one capture per open hook row of the session and clears the
// rows whose dialog is gone or replaced, so rows the sampler stopped watching
// cannot starve the cap (Task 3.3g). It is one pass, not a poll, and returns
// how many rows it cleared.
func (m *GuidanceHookMirror) reverifyOpen(ctx context.Context, sessionUUID string) int {
	if m.capture == nil {
		return 0
	}
	rows, err := m.store.ListOpenGuidanceRequestsByAsker(ctx, sessionUUID, domain.QuestionSourceHook)
	if err != nil {
		log.Warn("[GuidanceMirror] cap re-verification could not list rows", "session", sessionUUID, "err", err)
		return 0
	}
	cleared := 0
	for _, row := range rows {
		capture, cerr := m.capture(ctx, sessionUUID)
		reason, terminal := judgeRow(row, capture, cerr).cancelReason()
		if !terminal {
			continue
		}
		if err := m.Clear(ctx, row.ID, reason); err != nil {
			log.Warn("[GuidanceMirror] cap re-verification clear failed", "row", row.ID, "err", err)
			continue
		}
		cleared++
	}
	return cleared
}

// ClearOpenForSession clears every open hook row of a session: a PostToolUse
// receipt for AskUserQuestion means its current dialog was answered in the
// terminal, and any older open row's dialog is necessarily gone too. Rows
// already answered through the pane are not open, so they are untouched.
func (m *GuidanceHookMirror) ClearOpenForSession(ctx context.Context, sessionUUID string, reason domain.CancelReason) error {
	rows, err := m.store.ListOpenGuidanceRequestsByAsker(ctx, sessionUUID, domain.QuestionSourceHook)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if cerr := m.Clear(ctx, row.ID, reason); cerr != nil {
			return cerr
		}
	}
	return nil
}

// PurgeTerminalQuestions blanks questions_json of rows that ended over 30 days ago.
func (m *GuidanceHookMirror) PurgeTerminalQuestions(ctx context.Context, now time.Time) (int, error) {
	return m.store.PurgeTerminalGuidanceQuestions(ctx, now.Add(-session.GuidanceQuestionsRetention))
}
