package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/session"
)

// steerEnv is a SessionService with an audit sink on an in-memory file system,
// a delivery gate (for the counters) and a poller holding hidden targets.
type steerEnv struct {
	t    *testing.T
	fix  *forkTestFixture
	mfs  *memFS
	sink *AuditSink
	gate *deliverygate.Gate
	warn func(string) int
}

type steerTarget struct {
	inst        *session.Instance
	pm          *steerRecordingPM
	itemSession string
}

func newSteerEnv(t *testing.T) *steerEnv {
	t.Helper()
	fix := setupForkTestFixture(t)
	lg, warns := newLogCapture()
	mfs := newMemFS()
	sink := NewAuditSink(func() (string, error) { return "/cfg", nil }, WithAuditFS(mfs), WithAuditLogger(lg))
	t.Cleanup(sink.Close)
	fix.svc.featureFlagSvc.SetAudit(sink, nil)
	gate := deliverygate.NewGate()
	fix.svc.deliveryGate = gate
	return &steerEnv{t: t, fix: fix, mfs: mfs, sink: sink, gate: gate, warn: warns}
}

// addSession registers a started instance on a recording pane. role "" leaves
// it with no item_session row.
func (e *steerEnv) addSession(title string, hidden bool, role string, tweak ...func(*session.Instance)) *steerTarget {
	e.t.Helper()
	pm := &steerRecordingPM{}
	inst := session.NewStartedInstanceForTest(e.t, title, pm)
	inst.UUID = title + "-uuid"
	inst.Path = e.t.TempDir()
	inst.Status = session.Active
	inst.Program = "claude"
	inst.Hidden = hidden
	inst.CreatedAt, inst.UpdatedAt = time.Now(), time.Now()
	for _, f := range tweak {
		f(inst)
	}
	addInstanceToPoller(e.fix.poller, inst)
	tgt := &steerTarget{inst: inst, pm: pm}
	if role != "" {
		tgt.itemSession = e.linkRow(inst, role)
	}
	return tgt
}

func (e *steerEnv) linkRow(inst *session.Instance, role string) string {
	e.t.Helper()
	ctx := context.Background()
	item, err := e.fix.storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title: "Item for " + inst.Title, Status: string(session.BacklogStatusReview),
	})
	require.NoError(e.t, err)
	row, err := e.fix.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID: item.ID, SessionUUID: inst.UUID, SessionRole: role,
	})
	require.NoError(e.t, err)
	return row.ID
}

func (e *steerEnv) endRow(id string) {
	e.t.Helper()
	require.NoError(e.t, e.fix.storage.UpdateItemSessionEnded(context.Background(), id, time.Now()))
}

// requestCtx is the context the real server chain gives a handler.
func (e *steerEnv) requestCtx(listener string, requiresAuth bool, host string, hdr map[string]string) context.Context {
	cfg := verdictServiceConfig("0.0.0.0:8543", []string{"onyx.lan"})
	return stampedContext(e.t, &cfg, listener, requiresAuth, host, hdr)
}

func (e *steerEnv) localCtx() context.Context {
	return e.requestCtx(ListenerLocal, false, "localhost:8543", nil)
}

func (e *steerEnv) steer(ctx context.Context, id, msg string) error {
	_, err := e.fix.svc.UpdateSession(ctx, connect.NewRequest(&sessionv1.UpdateSessionRequest{Id: id, SteerMessage: &msg}))
	return err
}

// auditLines flushes the sink and returns every line, oldest first.
func (e *steerEnv) auditLines() []AuditLine {
	e.t.Helper()
	e.sink.Close()
	raw, ok := e.mfs.get(auditTestFile)
	if !ok || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var out []AuditLine
	for _, l := range lines(raw) {
		var al AuditLine
		require.NoError(e.t, json.Unmarshal(l, &al), string(l))
		out = append(out, al)
	}
	return out
}

func (e *steerEnv) count(outcome string) uint64 {
	return e.gate.Metrics().Value(deliverygate.CounterBacklogSteer, outcome)
}

func steerLines(all []AuditLine, kind string) []AuditLine {
	var out []AuditLine
	for _, l := range all {
		if l.Kind == kind {
			out = append(out, l)
		}
	}
	return out
}

// T-RO-23
func TestBacklogSteer_ShouldSucceedWithOneAuditPairAndMetric_WhenHiddenLiveLinkedReviewAndFailPreconditionWithZeroWrites_WhenTriageDiagnoseOtherUnlinkedOrEnded(t *testing.T) {
	e := newSteerEnv(t)
	review := e.addSession("review-ok", true, session.SessionRoleReview)
	const msg = "re-check the acceptance criteria"

	require.NoError(t, e.steer(e.localCtx(), review.inst.Title, msg))
	assert.Equal(t, []string{msg, session.EnterKeySequence}, review.pm.writes())
	assert.EqualValues(t, 1, e.count(steerOutcomeSent))
	session.AssertLeaseFree(t, review.inst)

	for _, role := range []string{session.SessionRoleDiagnose, session.SessionRoleTriage, session.SessionRoleExternal, ""} {
		tgt := e.addSession("hidden-"+role+"-x", true, role)
		err := e.steer(e.localCtx(), tgt.inst.Title, msg)
		require.Error(t, err, role)
		assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), role)
		assert.Empty(t, tgt.pm.writes(), "role %q: 0 writes", role)
	}
	ended := e.addSession("review-ended", true, session.SessionRoleReview)
	e.endRow(ended.itemSession)
	err := e.steer(e.localCtx(), ended.inst.Title, msg)
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Empty(t, ended.pm.writes())
	assert.EqualValues(t, 5, e.count(steerOutcomeNoLink))

	all := steerLines(e.auditLines(), auditKindBacklogSteer)
	require.Len(t, all, 2, "exactly one requested line and one result line, for the one qualifying steer")
	req, res := all[0], all[1]
	assert.Equal(t, auditPhaseRequest, req.Phase)
	assert.Equal(t, auditPhaseResult, res.Phase)
	assert.Equal(t, steerOutcomeSent, res.Outcome)
	assert.Equal(t, req.ChangeID, res.ChangeID)
	assert.Equal(t, review.inst.UUID, req.SessionUUID)
	assert.Equal(t, "review-ok", req.SessionTitle)
	assert.NotEmpty(t, req.ItemID)
	assert.Equal(t, len(msg), req.MessageLen)
	assert.Len(t, req.MessageSHA256, 64)
	assert.Equal(t, msg, req.MessagePreview)
	assert.Equal(t, ListenerLocal, req.Listener)
	assert.Equal(t, "localhost:8543", req.Host)
	assert.Equal(t, AuthModeNone, req.AuthMode)
	require.NotNil(t, req.PeerLoopback)
	require.NotNil(t, req.Proxied)
}

func TestBacklogSteer_ShouldRecordOnlyThe80CharacterPreview_WhenTheMessageIsLong(t *testing.T) {
	e := newSteerEnv(t)
	review := e.addSession("review-long", true, session.SessionRoleReview)
	msg := strings.Repeat("a", 79) + "é" + strings.Repeat("b", 500)

	require.NoError(t, e.steer(e.localCtx(), review.inst.Title, msg))
	req := steerLines(e.auditLines(), auditKindBacklogSteer)[0]
	assert.Equal(t, steerPreviewRunes, len([]rune(req.MessagePreview)))
	assert.Equal(t, len(msg), req.MessageLen)
}

// T-RO-25
func TestBacklogSteer_ShouldReturnInternalWithZeroWritesAndAuditFailedMetric_WhenPreWriteAuditAppendFails(t *testing.T) {
	e := newSteerEnv(t)
	review := e.addSession("review-audit", true, session.SessionRoleReview)
	e.mfs.failWrite = errors.New("disk full")

	err := e.steer(e.localCtx(), review.inst.Title, "go")
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.Contains(t, strings.ToLower(err.Error()), "audit log unavailable")
	assert.Empty(t, review.pm.writes())
	assert.EqualValues(t, 1, e.count(steerOutcomeAuditFailed))
	session.AssertLeaseFree(t, review.inst)
}

// T-RO-27
func TestBacklogReviewLink_ShouldKeyOnNewestDurableItemSessionRoleReviewAndEndedAtNilNotOnTags_WhenTagRemovedTagAddedOrNewestRowIsAnotherRole(t *testing.T) {
	e := newSteerEnv(t)
	ctx := context.Background()
	resolve := func(tgt *steerTarget) bool {
		_, ok, err := e.fix.svc.backlogLinks.ResolveLiveReviewLink(ctx, tgt.inst)
		require.NoError(t, err)
		return ok
	}

	untagged := e.addSession("review-untagged", true, session.SessionRoleReview)
	require.NoError(t, untagged.inst.SetTags(nil))
	assert.True(t, resolve(untagged), "a review row links with no backlog:review tag")

	tagged := e.addSession("diagnose-tagged", true, session.SessionRoleDiagnose)
	require.NoError(t, tagged.inst.SetTags([]string{"backlog:review"}))
	assert.False(t, resolve(tagged), "the tag does not make a diagnose row a review link")

	// The newest row wins: a review row followed by a diagnose row is not a review link.
	both := e.addSession("review-then-diagnose", true, session.SessionRoleReview)
	e.linkRow(both.inst, session.SessionRoleDiagnose)
	assert.False(t, resolve(both))

	// A tag edit in the same request never changes eligibility.
	tags := []string{"backlog:review", "x"}
	_, err := e.fix.svc.UpdateSession(e.localCtx(), connect.NewRequest(&sessionv1.UpdateSessionRequest{Id: untagged.inst.Title, Tags: tags}))
	require.NoError(t, err)
	assert.True(t, resolve(untagged))
}

// T-RO-28
func TestBacklogSteer_ShouldReturnNoLinkWithZeroWrites_WhenInstanceStoppedEndedAtNotYetWrittenOrSessionEndsBetweenFirstCheckAndPostAuditRecheck(t *testing.T) {
	t.Run("instance stopped before EndedAt is written", func(t *testing.T) {
		e := newSteerEnv(t)
		tgt := e.addSession("review-stopped", true, session.SessionRoleReview)
		tgt.inst.Status = session.Stopped

		err := e.steer(e.localCtx(), tgt.inst.Title, "go")
		require.Error(t, err)
		assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
		assert.Empty(t, tgt.pm.writes())
	})
	t.Run("ends between the first check and the post-audit re-check", func(t *testing.T) {
		e := newSteerEnv(t)
		tgt := e.addSession("review-racing", true, session.SessionRoleReview)
		real := e.fix.svc.backlogLinks
		calls := 0
		e.fix.svc.backlogLinks = linkResolverFunc(func(ctx context.Context, inst *session.Instance) (BacklogReviewLink, bool, error) {
			calls++
			if calls > 1 {
				return BacklogReviewLink{}, false, nil
			}
			return real.ResolveLiveReviewLink(ctx, inst)
		})

		err := e.steer(e.localCtx(), tgt.inst.Title, "go")
		require.Error(t, err)
		assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
		assert.Empty(t, tgt.pm.writes())
		assert.EqualValues(t, 1, e.count(steerOutcomeNoLink))
		session.AssertLeaseFree(t, tgt.inst)

		all := steerLines(e.auditLines(), auditKindBacklogSteer)
		require.Len(t, all, 2)
		assert.Equal(t, steerOutcomeAborted, all[1].Outcome)
	})
}

type linkResolverFunc func(ctx context.Context, inst *session.Instance) (BacklogReviewLink, bool, error)

func (f linkResolverFunc) ResolveLiveReviewLink(ctx context.Context, inst *session.Instance) (BacklogReviewLink, bool, error) {
	return f(ctx, inst)
}

// T-RO-33
func TestBacklogSteer_ShouldRejectEscC1BidiAndU2028CharactersButAcceptLfAndTabUpTo10000Bytes_WhenSteerMessageValidated(t *testing.T) {
	e := newSteerEnv(t)
	review := e.addSession("review-text", true, session.SessionRoleReview)

	for name, bad := range map[string]string{
		"esc":  "go\x1b[31m",
		"c1":   "go\u0085on",
		"bidi": "go\u202eon",
		"ls":   "go\u2028on",
		"cr":   "go\ron",
		"nul":  "go\x00on",
	} {
		err := e.steer(e.localCtx(), review.inst.Title, bad)
		require.Error(t, err, name)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), name)
	}
	assert.Empty(t, review.pm.writes())

	require.NoError(t, e.steer(e.localCtx(), review.inst.Title, "line one\n\tindented\u200dzwj"))
	assert.Len(t, review.pm.writes(), 2)

	exact := strings.Repeat("x", session.MaxSteerMessageLength)
	require.NoError(t, validateBacklogSteerText(exact))
	require.Error(t, validateBacklogSteerText(exact+"x"))
}

// T-RO-34
func TestBacklogSteer_ShouldReturnPermissionDeniedWithZeroWritesOnHostRebindingOrForeignOrigin_AndStillSteerAVerifiedLanHostnameBrowserOnAWildcardBindWithAuthOff_AndOnTheAuthenticatedChainWithAnUndetectedHostOrAnIpLiteralHost(t *testing.T) {
	e := newSteerEnv(t)
	review := e.addSession("review-host", true, session.SessionRoleReview)

	denied := []struct {
		name string
		ctx  context.Context
	}{
		{"rebinding host", e.requestCtx(ListenerLocal, false, "evil.example:8543", nil)},
		{"unverified lan host", e.requestCtx(ListenerLocal, false, "attacker.lan:8543", nil)},
		{"foreign origin", e.requestCtx(ListenerLocal, false, "onyx.lan:8543", map[string]string{"Origin": "https://evil.example"})},
	}
	for _, c := range denied {
		err := e.steer(c.ctx, review.inst.Title, "go")
		require.Error(t, err, c.name)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), c.name)
	}
	assert.Empty(t, review.pm.writes())
	assert.EqualValues(t, 3, e.count(steerOutcomeRefused))

	require.NoError(t, e.steer(e.requestCtx(ListenerLocal, false, "onyx.lan:8543", nil), review.inst.Title, "from the lan"))
	require.NoError(t, e.steer(e.requestCtx(ListenerRemote, true, "onyx.staplerhome.internal:8444", nil), review.inst.Title, "from the tailnet"))
	assert.Len(t, review.pm.writes(), 4)

	req := steerLines(e.auditLines(), auditKindBacklogSteer)[0]
	require.NotNil(t, req.PeerLoopback)
	assert.False(t, *req.PeerLoopback, "the audit line records the peer instead of refusing on it")
}

// T-RO-26
func TestUpdateSession_ShouldDecideSteerAccessFirstAndRejectSteerCombinedWithOtherFieldsForHidden_LeavingTitleTagsCategoryAndNoteUnchanged(t *testing.T) {
	e := newSteerEnv(t)
	review := e.addSession("review-alone", true, session.SessionRoleReview)
	review.inst.Category = "orig-cat"
	review.inst.Note = "orig note"
	steer, title, cat, note := "go", "renamed", "new-cat", "new note"

	_, err := e.fix.svc.UpdateSession(e.localCtx(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id: review.inst.Title, SteerMessage: &steer, Title: &title, Category: &cat, Note: &note, Tags: []string{"t1"},
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "steer_must_be_alone")
	assert.Equal(t, "review-alone", review.inst.Title)
	assert.Equal(t, "orig-cat", review.inst.Category)
	assert.Equal(t, "orig note", review.inst.Note)
	assert.Empty(t, review.inst.Tags)
	assert.Empty(t, review.pm.writes())

	// A hidden non-qualifying target is refused before any other field applies.
	tri := e.addSession("triage-x", true, session.SessionRoleTriage)
	_, err = e.fix.svc.UpdateSession(e.localCtx(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id: tri.inst.Title, SteerMessage: &steer, Note: &note,
	}))
	require.Error(t, err)
	assert.Empty(t, tri.inst.Note)
}

// T-RO-15
func TestUpdateSessionSteer_ShouldRejectHiddenTriageDiagnoseOtherUnlinkedOrEndedButReadRPCsAndSteerActiveSessionSucceed(t *testing.T) {
	e := newSteerEnv(t)
	tgt := e.addSession("hidden-triage-read", true, session.SessionRoleTriage)

	err := e.steer(e.localCtx(), tgt.inst.Title, "go")
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Empty(t, tgt.pm.writes())

	_, err = e.fix.svc.GetSession(context.Background(), connect.NewRequest(&sessionv1.GetSessionRequest{Id: tgt.inst.Title}))
	require.NoError(t, err, "read RPCs stay open")

	e.fix.svc.guardedSteer.ready = func(*session.Instance) notReadyReason { return notReadyNone }
	require.NoError(t, e.fix.svc.SteerActiveSession(context.Background(), tgt.inst.UUID, "internal go"))
	assert.Equal(t, []string{"internal go", session.EnterKeySequence}, tgt.pm.writes(), "the internal steer is characterized and unchanged")
}

func TestUpdateSessionSteer_ShouldBeUnchanged_WhenTheTargetIsVisible(t *testing.T) {
	e := newSteerEnv(t)
	vis := e.addSession("visible-steer", false, "")

	require.NoError(t, e.steer(context.Background(), vis.inst.Title, "hello"), "no request record is needed for a visible target")
	assert.Equal(t, []string{"hello", session.EnterKeySequence}, vis.pm.writes())
	assert.Empty(t, e.auditLines(), "a visible steer writes no audit line")
}

// T-RO-37
func TestSteerAuthorized_ShouldRefuseAndWriteNothing_WhenTokenInstanceUuidDiffersFromTargetOrKindIsWrong(t *testing.T) {
	e := newSteerEnv(t)
	a := e.addSession("steer-a", false, "")
	b := e.addSession("steer-b", false, "")
	ctx := context.Background()
	acquire := func(inst *session.Instance) *session.HeldLease {
		l, ok := inst.TryTerminalWriteLease(session.LeaseWriterSteer)
		require.True(t, ok)
		return l
	}

	cases := map[string]struct {
		auth  steerAuthorization
		inst  *session.Instance
		lease func() *session.HeldLease
		want  error
	}{
		"token for another instance": {internalSteerAuthorization(a.inst), b.inst, func() *session.HeldLease { return acquire(b.inst) }, errSteerAuthTargetDiffer},
		"zero token":                 {steerAuthorization{}, b.inst, func() *session.HeldLease { return acquire(b.inst) }, errSteerNoAuthorization},
		"lease of another instance":  {internalSteerAuthorization(b.inst), b.inst, func() *session.HeldLease { return acquire(a.inst) }, session.ErrLeaseMismatch},
		"nil lease":                  {internalSteerAuthorization(b.inst), b.inst, func() *session.HeldLease { return nil }, session.ErrNoLease},
	}
	for name, c := range cases {
		err := e.fix.svc.steerAuthorized(ctx, c.auth, c.lease(), c.inst, "x")
		require.ErrorIs(t, err, c.want, name)
	}
	assert.Empty(t, a.pm.writes())
	assert.Empty(t, b.pm.writes())
	session.AssertLeaseFree(t, a.inst)
	session.AssertLeaseFree(t, b.inst)

	// Each kind steers its own instance.
	for _, auth := range []steerAuthorization{
		internalSteerAuthorization(a.inst),
		{kind: steerAuthVisible, instanceUUID: a.inst.LeaseOwnerUUID()},
		{kind: steerAuthBacklogLink, instanceUUID: a.inst.LeaseOwnerUUID()},
	} {
		require.NoError(t, e.fix.svc.steerAuthorized(ctx, auth, acquire(a.inst), a.inst, "ok"))
	}
	assert.Len(t, a.pm.writes(), 6)
}

// T-RO-43
func TestSteerChains_ShouldWriteOnceOnAFreeLeaseAndReturnRetryableBusyOnAHeldLeaseWithoutAnyLinkReturningBusyToItself_WhenUpdateSessionSteerBacklogSteerAndSteerActiveSessionRun(t *testing.T) {
	e := newSteerEnv(t)
	vis := e.addSession("chain-visible", false, "")
	review := e.addSession("chain-review", true, session.SessionRoleReview)
	internal := e.addSession("chain-internal", true, "")

	chains := map[string]struct {
		tgt *steerTarget
		run func() error
	}{
		"UpdateSession -> steerUnderLease": {vis, func() error { return e.steer(context.Background(), vis.inst.Title, "m") }},
		"UpdateSession -> backlog link":    {review, func() error { return e.steer(e.localCtx(), review.inst.Title, "m") }},
		"SteerActiveSession -> internal": {internal, func() error {
			return e.fix.svc.SteerActiveSession(context.Background(), internal.inst.UUID, "m")
		}},
	}
	for name, c := range chains {
		held, ok := c.tgt.inst.TryTerminalWriteLease(session.LeaseWriterDriver)
		require.True(t, ok, name)
		err := c.run()
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), session.ErrLeaseBusy.Error(), name)
		assert.Empty(t, c.tgt.pm.writes(), "%s: 0 writes while the lease is held", name)
		held.Release()

		require.NoError(t, c.run(), name)
		assert.Equal(t, []string{"m", session.EnterKeySequence}, c.tgt.pm.writes(), "%s: exactly one write on a free lease", name)
		session.AssertLeaseFree(t, c.tgt.inst)
	}
}

// Failures map to the codes the composer reads.
func TestSteerErrorToConnect_ShouldMapAuditBusyAndInvalid(t *testing.T) {
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(steerErrorToConnect(errSteerAuditUnavailable)))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(steerErrorToConnect(validateBacklogSteerText("a\x1bb"))))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(steerErrorToConnect(session.ErrLeaseBusy)))
	assert.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(steerErrorToConnect(context.DeadlineExceeded)))
}
