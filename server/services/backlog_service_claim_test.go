package services

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	ssqlog "github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

const claimIssueURL = "https://github.com/acme/widgets/issues/42"

// fakeClaimChecker returns a canned verdict and counts calls per path.
type fakeClaimChecker struct {
	verdict    ClaimVerdict
	err        error
	liveCalls  atomic.Int32
	localCalls atomic.Int32
}

func (f *fakeClaimChecker) CheckClaim(context.Context, string) (ClaimVerdict, error) {
	f.liveCalls.Add(1)
	return f.verdict, f.err
}

func (f *fakeClaimChecker) CheckClaimLocalOnly(context.Context, string) (ClaimVerdict, error) {
	f.localCalls.Add(1)
	return f.verdict, f.err
}

func (f *fakeClaimChecker) ListForeignClaims() []session.ClaimRecord {
	f.localCalls.Add(1)
	if f.err == nil && f.verdict.Kind == ClaimHeldByOther {
		return []session.ClaimRecord{f.verdict.Record}
	}
	return nil
}

func heldByHostA(t *testing.T) ClaimVerdict {
	t.Helper()
	hostA := newClaimTestIdentity(t)
	return NewHeldByOtherVerdict(session.NewSignedClaimRecord(hostA, claimIssueURL, "ssq://hostA/backlog/v1/bl_1", time.Now()))
}

func captureClaimLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := ssqlog.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { ssqlog.SetSlogDefaultForTest(prev) })
	return &buf
}

func claimEnabledService(t *testing.T, checker CrossHostClaimChecker) *BacklogService {
	t.Helper()
	svc := NewBacklogService(createTestStorage(t), &mockSessionCreator{}, nil, nil, nil, nil)
	svc.claimDedupFlag = func() bool { return true }
	svc.SetClaimChecker(checker, nil)
	return svc
}

func serveClaimIssue(t *testing.T) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number": 42, "title": "Claimed issue", "body": "b", "state": "open", "html_url": claimIssueURL,
		})
	}))
	t.Cleanup(ts.Close)
	t.Cleanup(resetGhBaseURL(ts))
	t.Setenv("GITHUB_TOKEN", "fake-token")
}

func importClaimIssue(svc *BacklogService, override bool, reason string) (*connect.Response[sessionv1.ImportGitHubIssueResponse], error) {
	return svc.ImportGitHubIssue(context.Background(), connect.NewRequest(&sessionv1.ImportGitHubIssueRequest{
		IssueUrl: claimIssueURL, RepoPath: "/tmp/widgets", SkipPlanning: true,
		Override: override, OverrideReason: reason,
	}))
}

func itemCount(t *testing.T, svc *BacklogService) int {
	t.Helper()
	items, err := svc.storage.ListBacklogItems(context.Background(), session.BacklogItemFilter{})
	require.NoError(t, err)
	return len(items)
}

func TestImportGitHubIssue_should_ReturnAlreadyClaimedElsewhereWithNoRowCreated_When_ClaimHeldByOtherAndOverrideFalse(t *testing.T) {
	serveClaimIssue(t)
	checker := &fakeClaimChecker{verdict: heldByHostA(t)}
	svc := claimEnabledService(t, checker)

	resp, err := importClaimIssue(svc, false, "")

	require.NoError(t, err, "a claim is a structured outcome, not a connect.CodeAlreadyExists error")
	assert.Nil(t, resp.Msg.Item)
	claimed := resp.Msg.AlreadyClaimedElsewhere
	require.NotNil(t, claimed)
	assert.Equal(t, checker.verdict.Record.ClaimingHostID.String(), claimed.ClaimingHostId)
	assert.Equal(t, "ssq://hostA/backlog/v1/bl_1", claimed.ItemDeepLink)
	assert.Equal(t, claimIssueURL, claimed.ExternalUrl)
	assert.Zero(t, itemCount(t, svc))
}

func TestImportGitHubIssue_should_CreateItemAndLogOverride_When_OverrideTrueDespiteClaimHeldByOther(t *testing.T) {
	serveClaimIssue(t)
	logs := captureClaimLogs(t)
	checker := &fakeClaimChecker{verdict: heldByHostA(t)}
	svc := claimEnabledService(t, checker)

	resp, err := importClaimIssue(svc, true, "stale claim from crashed host")

	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Item)
	assert.Nil(t, resp.Msg.AlreadyClaimedElsewhere)
	assert.Equal(t, 1, itemCount(t, svc))
	out := logs.String()
	assert.Contains(t, out, "import_github_issue.claim_override")
	assert.Contains(t, out, claimIssueURL)
	assert.Contains(t, out, checker.verdict.Record.ClaimingHostID.String())
	assert.Contains(t, out, "stale claim from crashed host")
}

func TestImportGitHubIssue_should_ReturnInvalidArgument_When_OverrideTrueWithReasonUnderFiveCharacters(t *testing.T) {
	serveClaimIssue(t)
	svc := claimEnabledService(t, &fakeClaimChecker{verdict: heldByHostA(t)})

	for _, reason := range []string{"", "    ", "abcd", "  ab "} {
		_, err := importClaimIssue(svc, true, reason)
		var connectErr *connect.Error
		require.ErrorAs(t, err, &connectErr, "reason %q", reason)
		assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code(), "reason %q", reason)
	}
	assert.Zero(t, itemCount(t, svc))
}

func TestImportGitHubIssue_should_ProceedAndLogIndeterminate_When_ClaimCheckTimesOutAgainstAllPeers(t *testing.T) {
	serveClaimIssue(t)
	logs := captureClaimLogs(t)
	svc := claimEnabledService(t, &fakeClaimChecker{verdict: NewIndeterminateVerdict()})

	resp, err := importClaimIssue(svc, false, "")

	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Item)
	assert.Equal(t, 1, itemCount(t, svc))
	assert.Contains(t, logs.String(), "import_github_issue.claim_check_indeterminate")
}

func TestImportGitHubIssue_should_NotConsultChecker_When_FeatureFlagOff(t *testing.T) {
	serveClaimIssue(t)
	checker := &fakeClaimChecker{verdict: heldByHostA(t)}
	svc := claimEnabledService(t, checker)
	svc.claimDedupFlag = func() bool { return false }

	resp, err := importClaimIssue(svc, false, "")

	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Item, "flag off: behaves exactly as before the feature")
	assert.Zero(t, checker.liveCalls.Load())
	assert.Zero(t, checker.localCalls.Load())
}

func TestImportGitHubIssue_should_CreateItem_When_NoCheckerWired(t *testing.T) {
	serveClaimIssue(t)
	svc := NewBacklogService(createTestStorage(t), nil, nil, nil, nil, nil)
	svc.claimDedupFlag = func() bool { return true }

	resp, err := importClaimIssue(svc, false, "")

	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Item)
}

func createClaimTestItem(t *testing.T, svc *BacklogService, override bool, reason string) (*connect.Response[sessionv1.CreateBacklogItemResponse], error) {
	t.Helper()
	url := claimIssueURL
	return svc.CreateBacklogItem(context.Background(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "chat item", ExternalUrl: &url, SkipTriage: true,
		OverrideClaim: override, OverrideReason: reason,
	}))
}

func TestCreateBacklogItem_should_ReturnAlreadyClaimedElsewhereAndCreateNoRow_When_ExternalURLHeldByOtherAndOverrideFalse(t *testing.T) {
	svc := claimEnabledService(t, &fakeClaimChecker{verdict: heldByHostA(t)})

	resp, err := createClaimTestItem(t, svc, false, "")

	require.NoError(t, err)
	assert.Nil(t, resp.Msg.Item)
	require.NotNil(t, resp.Msg.AlreadyClaimedElsewhere)
	assert.Equal(t, "ssq://hostA/backlog/v1/bl_1", resp.Msg.AlreadyClaimedElsewhere.ItemDeepLink)
	assert.Zero(t, itemCount(t, svc))
}

func TestCreateBacklogItem_should_CreateAndLogNlCreateClaimOverride_When_OverrideTrueWithValidReason(t *testing.T) {
	logs := captureClaimLogs(t)
	svc := claimEnabledService(t, &fakeClaimChecker{verdict: heldByHostA(t)})

	resp, err := createClaimTestItem(t, svc, true, "I own this one")

	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Item)
	assert.Equal(t, 1, itemCount(t, svc))
	assert.Contains(t, logs.String(), "nl_create.claim_override")
	assert.Contains(t, logs.String(), "I own this one")

	_, err = createClaimTestItem(t, svc, true, "no")
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestCreateBacklogItem_should_ProceedAndLogIndeterminate_When_ClaimCheckTimesOutForChatCreatedItem(t *testing.T) {
	logs := captureClaimLogs(t)
	svc := claimEnabledService(t, &fakeClaimChecker{verdict: NewIndeterminateVerdict()})

	resp, err := createClaimTestItem(t, svc, false, "")

	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Item)
	assert.Contains(t, logs.String(), "nl_create.claim_check_indeterminate")
}

func TestCreateBacklogItem_should_SkipClaimCheck_When_NoExternalURL(t *testing.T) {
	checker := &fakeClaimChecker{verdict: heldByHostA(t)}
	svc := claimEnabledService(t, checker)

	resp, err := svc.CreateBacklogItem(context.Background(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{Title: "plain", SkipTriage: true}))

	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Item)
	assert.Zero(t, checker.liveCalls.Load())
}

// createQueuedItemWithURL creates an item carrying externalURL and queues it.
func createQueuedItemWithURL(t *testing.T, svc *BacklogService, repoPath, externalURL string) string {
	t.Helper()
	// Creation would run the pre-creation claim gate; this helper is about
	// the dequeue path, so suspend the flag while seeding.
	flag := svc.claimDedupFlag
	svc.claimDedupFlag = func() bool { return false }
	defer func() { svc.claimDedupFlag = flag }()
	ctx := context.Background()
	resp, err := svc.CreateBacklogItem(ctx, connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "queued " + externalURL, RepoPath: repoPath, ExternalUrl: &externalURL,
		AcceptanceCriteria: []*sessionv1.AcCriterion{{Index: 0, Text: "test", Status: "pending"}},
		SkipTriage:         true, SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := resp.Msg.Item.Id
	_, err = svc.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{ItemId: itemID, TargetStatus: "ready"}))
	require.NoError(t, err)
	item, err := svc.storage.GetBacklogItem(ctx, itemID)
	require.NoError(t, err)
	_, err = svc.queueBacklogItem(ctx, item, false)
	require.NoError(t, err)
	return itemID
}

func itemStatus(t *testing.T, svc *BacklogService, itemID string) string {
	t.Helper()
	resp, err := svc.GetBacklogItem(context.Background(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	return resp.Msg.Item.Status
}

func openClaimStuckRows(t *testing.T, svc *BacklogService, itemID string) int {
	t.Helper()
	resp, err := svc.ListStuckBacklogItems(context.Background(), connect.NewRequest(&sessionv1.ListStuckBacklogItemsRequest{}))
	require.NoError(t, err)
	n := 0
	for _, it := range resp.Msg.Items {
		if it.ItemId == itemID && it.Reason == sessionv1.StuckReason_STUCK_REASON_BLOCKED_BY_CLAIM {
			n++
		}
	}
	return n
}

func newDequeueClaimService(t *testing.T, checker CrossHostClaimChecker) (*BacklogService, *mockSessionCreator, string) {
	t.Helper()
	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(createTestStorage(t), creator, nil, nil, nil, nil)
	svc.claimDedupFlag = func() bool { return true }
	svc.SetClaimChecker(checker, nil)
	return svc, creator, repoPath
}

func TestDequeueNextQueuedItems_should_DequeueNormally_When_ClaimIndexHasNoConflictingEntry(t *testing.T) {
	checker := &fakeClaimChecker{verdict: NewUnclaimedVerdict()}
	svc, _, repoPath := newDequeueClaimService(t, checker)
	itemID := createQueuedItemWithURL(t, svc, repoPath, claimIssueURL)

	require.NoError(t, svc.DequeueNextQueuedItems(context.Background()))

	assert.Equal(t, "in_progress", itemStatus(t, svc, itemID))
	assert.EqualValues(t, 1, checker.localCalls.Load())
	assert.Zero(t, checker.liveCalls.Load(), "the sweep never uses the live peer query")
}

func TestDequeueNextQueuedItems_should_SkipCandidateAndLogBlockedByClaim_When_LocalClaimIndexShowsDifferentClaimingHostID(t *testing.T) {
	logs := captureClaimLogs(t)
	checker := &fakeClaimChecker{verdict: heldByHostA(t)}
	svc, creator, repoPath := newDequeueClaimService(t, checker)
	itemID := createQueuedItemWithURL(t, svc, repoPath, claimIssueURL)

	require.NoError(t, svc.DequeueNextQueuedItems(context.Background()))

	assert.Equal(t, "queued", itemStatus(t, svc, itemID), "left at its current status")
	assert.Empty(t, creator.calls, "no session spawned")
	assert.Contains(t, logs.String(), "dequeue.blocked_by_claim")
	assert.Contains(t, logs.String(), checker.verdict.Record.ClaimingHostID.String())
}

func TestDequeueNextQueuedItems_should_CallMarkStuckWithBlockedByClaimReason_When_CandidateSkippedForDifferentHostClaim(t *testing.T) {
	svc, _, repoPath := newDequeueClaimService(t, &fakeClaimChecker{verdict: heldByHostA(t)})
	itemID := createQueuedItemWithURL(t, svc, repoPath, claimIssueURL)

	require.NoError(t, svc.DequeueNextQueuedItems(context.Background()))
	require.NoError(t, svc.DequeueNextQueuedItems(context.Background()), "a second sweep is idempotent")

	assert.Equal(t, 1, openClaimStuckRows(t, svc, itemID))
}

func TestDequeueNextQueuedItems_should_DequeueAsBefore_When_FeatureFlagOff(t *testing.T) {
	checker := &fakeClaimChecker{verdict: heldByHostA(t)}
	svc, _, repoPath := newDequeueClaimService(t, checker)
	svc.claimDedupFlag = func() bool { return false }
	itemID := createQueuedItemWithURL(t, svc, repoPath, claimIssueURL)

	require.NoError(t, svc.DequeueNextQueuedItems(context.Background()))

	assert.Equal(t, "in_progress", itemStatus(t, svc, itemID))
	assert.Zero(t, checker.localCalls.Load())
}

func TestDequeueNextQueuedItems_should_NeverIssueNetworkCallWhileDequeueMuHeld_When_CandidateHasExternalURL(t *testing.T) {
	self, peerID := newClaimTestIdentity(t), newClaimTestIdentity(t)
	peer, hits := claimPeer(t, peerID, claimingHandler(session.NewSignedClaimRecord(peerID, claimIssueURL, "ssq://p/x", time.Now())))
	local := NewLocalClaimChecker(newClaimTestIndex(t), self.ID, fakePeerLister{entries: []session.RegistryEntry{peer}})
	svc, _, repoPath := newDequeueClaimService(t, local)
	itemID := createQueuedItemWithURL(t, svc, repoPath, claimIssueURL)

	require.NoError(t, svc.DequeueNextQueuedItems(context.Background()))

	assert.Zero(t, hits.Load(), "the sweep consults only the local ClaimIndex")
	assert.Equal(t, "in_progress", itemStatus(t, svc, itemID), "an unpropagated remote claim is an accepted false negative")
}

func TestOverrideClaimBlock_should_DequeueAndClaimItemDespiteActiveClaim_When_ValidReasonGiven(t *testing.T) {
	logs := captureClaimLogs(t)
	checker := &fakeClaimChecker{verdict: heldByHostA(t)}
	svc, creator, repoPath := newDequeueClaimService(t, checker)
	itemID := createQueuedItemWithURL(t, svc, repoPath, claimIssueURL)
	require.NoError(t, svc.DequeueNextQueuedItems(context.Background()))
	require.Equal(t, 1, openClaimStuckRows(t, svc, itemID))

	resp, err := svc.OverrideClaimBlock(context.Background(), connect.NewRequest(&sessionv1.OverrideClaimBlockRequest{
		ItemId: itemID, Reason: "hostA crashed, this is mine",
	}))

	require.NoError(t, err)
	assert.Equal(t, "in_progress", resp.Msg.Item.Status)
	assert.Len(t, creator.calls, 1)
	assert.Zero(t, openClaimStuckRows(t, svc, itemID), "stuck row resolved")
	out := logs.String()
	assert.Contains(t, out, "dequeue.claim_override")
	assert.Contains(t, out, "hostA crashed, this is mine")
	assert.Contains(t, out, claimIssueURL)
	assert.Contains(t, out, checker.verdict.Record.ClaimingHostID.String())
}

func TestOverrideClaimBlock_should_ReturnInvalidArgument_When_ReasonUnderFiveCharacters(t *testing.T) {
	svc, creator, repoPath := newDequeueClaimService(t, &fakeClaimChecker{verdict: heldByHostA(t)})
	itemID := createQueuedItemWithURL(t, svc, repoPath, claimIssueURL)

	_, err := svc.OverrideClaimBlock(context.Background(), connect.NewRequest(&sessionv1.OverrideClaimBlockRequest{ItemId: itemID, Reason: "no"}))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
	assert.Equal(t, "queued", itemStatus(t, svc, itemID))
	assert.Empty(t, creator.calls)
}

func TestOverrideClaimBlock_should_ReturnFailedPrecondition_When_ItemNotQueuedOrReady(t *testing.T) {
	svc, _, _ := newDequeueClaimService(t, &fakeClaimChecker{})
	created, err := createClaimTestItem(t, svc, false, "")
	require.NoError(t, err)

	_, err = svc.OverrideClaimBlock(context.Background(), connect.NewRequest(&sessionv1.OverrideClaimBlockRequest{ItemId: created.Msg.Item.Id, Reason: "valid reason"}))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
}

type fakeDisputeResolver struct{ resolved []string }

func (f *fakeDisputeResolver) ResolveDispute(_ context.Context, url string) error {
	f.resolved = append(f.resolved, url)
	return nil
}

func TestResolveClaimDispute_should_DelegateToResolverAndRequireURL(t *testing.T) {
	logs := captureClaimLogs(t)
	svc := NewBacklogService(nil, nil, nil, nil, nil, nil)

	_, err := svc.ResolveClaimDispute(context.Background(), connect.NewRequest(&sessionv1.ResolveClaimDisputeRequest{ExternalUrl: claimIssueURL}))
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code(), "no resolver wired")

	resolver := &fakeDisputeResolver{}
	svc.SetClaimChecker(nil, resolver)
	_, err = svc.ResolveClaimDispute(context.Background(), connect.NewRequest(&sessionv1.ResolveClaimDisputeRequest{ExternalUrl: claimIssueURL, Reason: "looked, host B is right"}))
	require.NoError(t, err)
	assert.Equal(t, []string{claimIssueURL}, resolver.resolved)
	assert.Contains(t, logs.String(), "claim_dispute.resolved")

	_, err = svc.ResolveClaimDispute(context.Background(), connect.NewRequest(&sessionv1.ResolveClaimDisputeRequest{}))
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestStuckReasonBlockedByClaim_should_RoundTripProtoEnum_When_Marshaled(t *testing.T) {
	assert.True(t, domain.StuckReasonBlockedByClaim.IsValid())
	assert.Equal(t, sessionv1.StuckReason_STUCK_REASON_BLOCKED_BY_CLAIM, toProtoStuckReason(domain.StuckReasonBlockedByClaim))
	assert.Equal(t, domain.StuckReasonBlockedByClaim, fromProtoStuckReason(sessionv1.StuckReason_STUCK_REASON_BLOCKED_BY_CLAIM))
}

func checkClaimRPC(t *testing.T, svc *BacklogService, localOnly bool) *sessionv1.CheckCrossHostClaimResponse {
	t.Helper()
	resp, err := svc.CheckCrossHostClaim(context.Background(), connect.NewRequest(&sessionv1.CheckCrossHostClaimRequest{
		ExternalUrl: claimIssueURL, LocalOnly: localOnly,
	}))
	require.NoError(t, err)
	return resp.Msg
}

func TestCheckCrossHostClaim_should_ReturnClaimWithHostAndLink_When_HeldByOther(t *testing.T) {
	checker := &fakeClaimChecker{verdict: heldByHostA(t)}
	svc := claimEnabledService(t, checker)

	got := checkClaimRPC(t, svc, false)

	assert.True(t, got.Enabled)
	assert.True(t, got.Checked)
	require.NotNil(t, got.Claim)
	assert.Equal(t, "ssq://hostA/backlog/v1/bl_1", got.Claim.ItemDeepLink)
	assert.NotZero(t, got.ClaimedAtUnix)
	assert.EqualValues(t, 1, checker.liveCalls.Load())
}

func TestCheckCrossHostClaim_should_UseOnlyLocalIndex_When_LocalOnlySet(t *testing.T) {
	checker := &fakeClaimChecker{verdict: NewUnclaimedVerdict()}
	svc := claimEnabledService(t, checker)

	got := checkClaimRPC(t, svc, true)

	assert.True(t, got.Checked)
	assert.Nil(t, got.Claim)
	assert.Zero(t, checker.liveCalls.Load())
	assert.EqualValues(t, 1, checker.localCalls.Load())
}

func TestCheckCrossHostClaim_should_ReportUncheckedNeverClear_When_Indeterminate(t *testing.T) {
	svc := claimEnabledService(t, &fakeClaimChecker{verdict: NewIndeterminateVerdict()})

	got := checkClaimRPC(t, svc, false)

	assert.True(t, got.Enabled)
	assert.False(t, got.Checked)
	assert.Nil(t, got.Claim)
}

func TestCheckCrossHostClaim_should_ReportDisabledAndSkipChecker_When_FeatureFlagOff(t *testing.T) {
	checker := &fakeClaimChecker{verdict: heldByHostA(t)}
	svc := claimEnabledService(t, checker)
	svc.claimDedupFlag = func() bool { return false }

	got := checkClaimRPC(t, svc, false)

	assert.False(t, got.Enabled)
	assert.Nil(t, got.Claim)
	assert.Zero(t, checker.liveCalls.Load()+checker.localCalls.Load())
}

func TestCheckCrossHostClaim_should_RequireExternalURL(t *testing.T) {
	svc := claimEnabledService(t, &fakeClaimChecker{})
	_, err := svc.CheckCrossHostClaim(context.Background(), connect.NewRequest(&sessionv1.CheckCrossHostClaimRequest{}))
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestListForeignClaims_should_ReturnOnlyOtherHostsClaims_When_FlagOn(t *testing.T) {
	self, other := newClaimTestIdentity(t), newClaimTestIdentity(t)
	index := newClaimTestIndex(t, other)
	for _, c := range []session.ClaimRecord{
		session.NewSignedClaimRecord(self, "https://github.com/acme/widgets/issues/1", "ssq://self/backlog/v1/a", time.Now()),
		session.NewSignedClaimRecord(other, "https://github.com/acme/widgets/issues/2", "ssq://other/backlog/v1/b", time.Now()),
	} {
		_, err := index.RecordClaim(c)
		require.NoError(t, err)
	}
	svc := claimEnabledService(t, NewLocalClaimChecker(index, self.ID, fakePeerLister{}))

	resp, err := svc.ListForeignClaims(context.Background(), connect.NewRequest(&sessionv1.ListForeignClaimsRequest{}))

	require.NoError(t, err)
	assert.True(t, resp.Msg.Enabled)
	require.Len(t, resp.Msg.Claims, 1)
	assert.Equal(t, "https://github.com/acme/widgets/issues/2", resp.Msg.Claims[0].ExternalUrl)
	assert.Equal(t, other.ID.String(), resp.Msg.Claims[0].ClaimingHostId)

	svc.claimDedupFlag = func() bool { return false }
	off, err := svc.ListForeignClaims(context.Background(), connect.NewRequest(&sessionv1.ListForeignClaimsRequest{}))
	require.NoError(t, err)
	assert.False(t, off.Msg.Enabled)
	assert.Empty(t, off.Msg.Claims)
}

func TestDequeueNextQueuedItems_should_ClearBlockedByClaimRow_When_ClaimNoLongerHeld(t *testing.T) {
	checker := &fakeClaimChecker{verdict: heldByHostA(t)}
	svc, _, repoPath := newDequeueClaimService(t, checker)
	itemID := createQueuedItemWithURL(t, svc, repoPath, claimIssueURL)
	require.NoError(t, svc.DequeueNextQueuedItems(context.Background()))
	require.Equal(t, 1, openClaimStuckRows(t, svc, itemID))

	checker.verdict = NewUnclaimedVerdict()
	require.NoError(t, svc.DequeueNextQueuedItems(context.Background()))

	assert.Zero(t, openClaimStuckRows(t, svc, itemID))
}

func TestDequeueNextQueuedItems_should_ClearBlockedByClaimRow_When_FlagTurnedOff(t *testing.T) {
	svc, _, repoPath := newDequeueClaimService(t, &fakeClaimChecker{verdict: heldByHostA(t)})
	itemID := createQueuedItemWithURL(t, svc, repoPath, claimIssueURL)
	require.NoError(t, svc.DequeueNextQueuedItems(context.Background()))
	require.Equal(t, 1, openClaimStuckRows(t, svc, itemID))

	svc.claimDedupFlag = func() bool { return false }
	require.NoError(t, svc.DequeueNextQueuedItems(context.Background()))

	assert.Zero(t, openClaimStuckRows(t, svc, itemID))
}

func TestDequeueNextQueuedItems_should_KeepBlockedByClaimRow_When_ClaimStillHeld(t *testing.T) {
	svc, _, repoPath := newDequeueClaimService(t, &fakeClaimChecker{verdict: heldByHostA(t)})
	itemID := createQueuedItemWithURL(t, svc, repoPath, claimIssueURL)
	require.NoError(t, svc.DequeueNextQueuedItems(context.Background()))
	require.NoError(t, svc.DequeueNextQueuedItems(context.Background()))

	assert.Equal(t, 1, openClaimStuckRows(t, svc, itemID))
}
