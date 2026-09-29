package session_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

const twoHostIssueURL = "https://github.com/acme/widgets/issues/42"

// twoHostNode is one simulated stapler-squad host: its own storage, claim
// index, gossiper and TLS claim endpoints (claimNode), the production
// ClaimIndexRecorder wired into that storage, and a BacklogService whose
// checker is the production LocalClaimChecker. Only the network is simulated.
type twoHostNode struct {
	*claimNode
	storage *session.Storage
	backlog *services.BacklogService
}

// newTwoHostNode builds a host. withGossip=false wires a recorder that never
// broadcasts, standing in for a claim that has not propagated yet.
func newTwoHostNode(t *testing.T, name string, withGossip bool) *twoHostNode {
	t.Helper()
	node := newClaimNode(t)
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	gossiper := node.gossiper
	if !withGossip {
		gossiper = nil
	}
	recorder, err := session.NewClaimIndexRecorder(node.identity, node.index, gossiper, name, nil)
	require.NoError(t, err)
	storage.SetClaimRecorder(recorder)

	backlog := services.NewBacklogService(storage, nil, nil, nil, nil, nil)
	checker := services.NewLocalClaimChecker(node.index, node.identity.ID, node.registry)
	backlog.SetClaimChecker(checker, checker)
	return &twoHostNode{claimNode: node, storage: storage, backlog: backlog}
}

func enableClaimDedupFlag(t *testing.T) {
	t.Helper()
	envtest.NewIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag("cross_host_claim_dedup", true))
}

func serveTwoHostIssue(t *testing.T) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number": 42, "title": "Shared issue", "body": "b", "state": "open", "html_url": twoHostIssueURL,
		})
	}))
	t.Cleanup(ts.Close)
	t.Cleanup(github.SetGhBaseURLForTest(ts.URL + "/"))
	t.Setenv("GITHUB_TOKEN", "fake-token")
}

func (n *twoHostNode) importIssue(t *testing.T, override bool, reason string) *sessionv1.ImportGitHubIssueResponse {
	t.Helper()
	resp, err := n.backlog.ImportGitHubIssue(context.Background(), connect.NewRequest(&sessionv1.ImportGitHubIssueRequest{
		IssueUrl: twoHostIssueURL, RepoPath: "/tmp/widgets", SkipPlanning: true,
		Override: override, OverrideReason: reason,
	}))
	require.NoError(t, err)
	return resp.Msg
}

func (n *twoHostNode) checkClaim(t *testing.T, localOnly bool) *sessionv1.CheckCrossHostClaimResponse {
	t.Helper()
	resp, err := n.backlog.CheckCrossHostClaim(context.Background(), connect.NewRequest(&sessionv1.CheckCrossHostClaimRequest{
		ExternalUrl: twoHostIssueURL, LocalOnly: localOnly,
	}))
	require.NoError(t, err)
	return resp.Msg
}

func (n *twoHostNode) itemCount(t *testing.T) int {
	t.Helper()
	items, err := n.storage.ListBacklogItems(context.Background(), session.BacklogItemFilter{})
	require.NoError(t, err)
	return len(items)
}

// TestTwoHostClaimLoop_should_BlockDuplicateImportOnHostB_When_HostAImportedFirst
// is the pre-mortem #3 scripted loop: record -> gossip -> check -> blocked
// import -> audited override, across two hosts joined only by real TLS endpoints.
func TestTwoHostClaimLoop_should_BlockDuplicateImportOnHostB_When_HostAImportedFirst(t *testing.T) {
	enableClaimDedupFlag(t)
	serveTwoHostIssue(t)
	a := newTwoHostNode(t, "hostA", true)
	b := newTwoHostNode(t, "hostB", true)
	a.learn(t, b.claimNode)
	b.learn(t, a.claimNode)

	// record: host A imports the issue, which records its claim.
	first := a.importIssue(t, false, "")
	require.NotNil(t, first.Item, "host A is first, so nothing blocks it")
	require.Nil(t, first.AlreadyClaimedElsewhere)

	// gossip: the claim reaches host B's index without B asking.
	require.Eventually(t, func() bool {
		claim, ok := b.index.CheckClaim(twoHostIssueURL)
		return ok && claim.ClaimingHostID.String() == a.identity.ID.String()
	}, 5*time.Second, 25*time.Millisecond, "host B's index never received host A's claim")

	// check: B reports the claim from its local index alone.
	local := b.checkClaim(t, true)
	require.True(t, local.Enabled)
	require.NotNil(t, local.Claim)
	assert.Equal(t, a.identity.ID.String(), local.Claim.ClaimingHostId)
	assert.Contains(t, local.Claim.ItemDeepLink, "ssq://hostA/backlog/v1/")

	// blocked: B's import returns the structured outcome and creates nothing.
	blocked := b.importIssue(t, false, "")
	assert.Nil(t, blocked.Item)
	require.NotNil(t, blocked.AlreadyClaimedElsewhere)
	assert.Equal(t, a.identity.ID.String(), blocked.AlreadyClaimedElsewhere.ClaimingHostId)
	assert.Zero(t, b.itemCount(t), "a blocked import must not create a row")

	// override: an audited reason lets B proceed; A's row is untouched.
	overridden := b.importIssue(t, true, "host A crashed and will not finish this")
	require.NotNil(t, overridden.Item)
	assert.Equal(t, 1, b.itemCount(t))
	assert.Equal(t, 1, a.itemCount(t))

	// dispute: two hosts now claim one URL; B's index flags it for a human
	// instead of silently picking a winner.
	claim, ok := b.index.CheckClaim(twoHostIssueURL)
	require.True(t, ok)
	assert.True(t, claim.Disputed)
}

// TestTwoHostClaimLoop_should_FindClaimViaLivePeerLookup_When_GossipHasNotArrived
// covers the case gossip cannot: the claim exists on A but never reached B, so
// B's local-only read is an accepted false negative while the live fan-out to
// A's /internal/claim-lookup still blocks the import.
func TestTwoHostClaimLoop_should_FindClaimViaLivePeerLookup_When_GossipHasNotArrived(t *testing.T) {
	enableClaimDedupFlag(t)
	serveTwoHostIssue(t)
	a := newTwoHostNode(t, "hostA", false)
	b := newTwoHostNode(t, "hostB", true)
	b.learn(t, a.claimNode)

	require.NotNil(t, a.importIssue(t, false, "").Item)

	local := b.checkClaim(t, true)
	assert.True(t, local.Checked)
	assert.Nil(t, local.Claim, "an unpropagated claim is invisible to the local-only read")

	live := b.checkClaim(t, false)
	require.NotNil(t, live.Claim, "the live peer query must find host A's claim")
	assert.Equal(t, a.identity.ID.String(), live.Claim.ClaimingHostId)

	blocked := b.importIssue(t, false, "")
	require.NotNil(t, blocked.AlreadyClaimedElsewhere)
	assert.Zero(t, b.itemCount(t))
}

// TestTwoHostClaimLoop_should_ReportUncheckedNotClear_When_OnlyKnownPeerIsDown
// pins the indeterminate branch end to end: an unreachable known peer must never
// read as "confirmed clear", yet must not block the import either.
func TestTwoHostClaimLoop_should_ReportUncheckedNotClear_When_OnlyKnownPeerIsDown(t *testing.T) {
	enableClaimDedupFlag(t)
	serveTwoHostIssue(t)
	a := newTwoHostNode(t, "hostA", false)
	b := newTwoHostNode(t, "hostB", true)
	b.learn(t, a.claimNode)
	a.closeServer()

	got := b.checkClaim(t, false)

	assert.True(t, got.Enabled)
	assert.False(t, got.Checked, "an unreachable peer means unknown, not clear")
	assert.Nil(t, got.Claim)
	require.NotNil(t, b.importIssue(t, false, "").Item, "an indeterminate check proceeds optimistically")
}
