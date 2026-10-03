package services

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
)

type countingCommentLister struct {
	comments []github.PRComment
	err      error
	calls    atomic.Int32
}

func (c *countingCommentLister) list(context.Context, github.RepoRef, int) ([]github.PRComment, error) {
	c.calls.Add(1)
	return c.comments, c.err
}

func provenanceComments(t *testing.T) ([]github.PRComment, session.HostID) {
	t.Helper()
	identity, err := session.LoadOrCreateHostIdentity(t.TempDir())
	require.NoError(t, err)
	stamp := session.FormatPRProvenanceComment(identity.ID, "ssq://laptop/backlog/v1/bl_01J7QK0000000000000000000A")
	return []github.PRComment{{Body: "lgtm"}, {Body: stamp}, {Body: "CI is red"}}, identity.ID
}

func newProvenanceWebhookHandler(t *testing.T, router PRFixEventRouter, lister *countingCommentLister, dedupFlag bool) (*GitHubWebhookHandler, *webhookTestInfra) {
	t.Helper()
	infra := newWebhookTestInfra(t)
	infra.cfg.FeatureFlags["pr_event_webhooks"] = true
	infra.cfg.FeatureFlags[crossHostClaimDedupFlagName] = dedupFlag
	newGitHubPushWorkflow(t, infra, "gh-prov", "s3cr3t", "tstapler/stapler-squad", "main", "x")
	h := NewGitHubWebhookHandler(infra.workflowRepo, infra.scheduler, infra.fireEvents, infra.cfg, router, nil)
	prev := provenanceReader
	provenanceReader = newPRProvenanceReader(lister.list)
	t.Cleanup(func() { provenanceReader = prev })
	return h, infra
}

func deliverCheckRun(t *testing.T, h *GitHubWebhookHandler, deliveryID string) {
	t.Helper()
	body := checkRunFailureBody(t)
	rec := doPRFixEventRequest(t, h, "check_run", body, deliveryID, sign("s3cr3t", body))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandlePRFixEvent_should_ResolveItemFromStampedComment_When_LocalLookupMisses(t *testing.T) {
	buf := captureLogs(t)
	comments, hostID := provenanceComments(t)
	lister := &countingCommentLister{comments: comments}
	h, _ := newProvenanceWebhookHandler(t, &fakePRFixEventRouter{matched: false}, lister, true)

	deliverCheckRun(t, h, "delivery-prov-miss")

	assert.EqualValues(t, 1, lister.calls.Load())
	assert.Contains(t, buf.String(), "github_webhook.pr_provenance_resolved")
	assert.Contains(t, buf.String(), "bl_01J7QK0000000000000000000A")
	assert.Contains(t, buf.String(), hostID.String())
}

func TestHandlePRFixEvent_should_NotReadComments_When_LocalLookupMatches(t *testing.T) {
	comments, _ := provenanceComments(t)
	lister := &countingCommentLister{comments: comments}
	h, _ := newProvenanceWebhookHandler(t, &fakePRFixEventRouter{matched: true}, lister, true)

	deliverCheckRun(t, h, "delivery-prov-hit")

	assert.Zero(t, lister.calls.Load(), "the fallback is additive: a local match never triggers it")
}

func TestHandlePRFixEvent_should_NotReadComments_When_ClaimDedupFlagOff(t *testing.T) {
	comments, _ := provenanceComments(t)
	lister := &countingCommentLister{comments: comments}
	h, _ := newProvenanceWebhookHandler(t, &fakePRFixEventRouter{matched: false}, lister, false)

	deliverCheckRun(t, h, "delivery-prov-off")

	assert.Zero(t, lister.calls.Load())
}

func TestPRProvenanceReader_should_CacheHitsAndMisses_ButRetryAfterErrors(t *testing.T) {
	ref, err := github.NewRepoRef("tstapler", "stapler-squad")
	require.NoError(t, err)
	now := time.Now()
	comments, _ := provenanceComments(t)

	hit := &countingCommentLister{comments: comments}
	reader := newPRProvenanceReader(hit.list)
	reader.now = func() time.Time { return now }
	for i := 0; i < 3; i++ {
		_, ok := reader.Resolve(context.Background(), ref, 7)
		assert.True(t, ok)
	}
	assert.EqualValues(t, 1, hit.calls.Load())
	now = now.Add(provenanceCacheTTL + time.Second)
	reader.Resolve(context.Background(), ref, 7)
	assert.EqualValues(t, 2, hit.calls.Load(), "an expired entry is re-read")

	miss := &countingCommentLister{comments: []github.PRComment{{Body: "no stamp"}}}
	reader = newPRProvenanceReader(miss.list)
	reader.Resolve(context.Background(), ref, 7)
	reader.Resolve(context.Background(), ref, 7)
	assert.EqualValues(t, 1, miss.calls.Load(), "a miss is cached too")

	failing := &countingCommentLister{err: errors.New("gh unavailable")}
	reader = newPRProvenanceReader(failing.list)
	_, ok := reader.Resolve(context.Background(), ref, 7)
	assert.False(t, ok)
	reader.Resolve(context.Background(), ref, 7)
	assert.EqualValues(t, 2, failing.calls.Load(), "errors are not cached")
}

func TestPRProvenanceReader_should_PreferNewestStamp_When_PRWasRestamped(t *testing.T) {
	ref, err := github.NewRepoRef("tstapler", "stapler-squad")
	require.NoError(t, err)
	identity, err := session.LoadOrCreateHostIdentity(t.TempDir())
	require.NoError(t, err)
	older := session.FormatPRProvenanceComment(identity.ID, "ssq://a/backlog/v1/bl_OLD")
	newer := session.FormatPRProvenanceComment(identity.ID, "ssq://a/backlog/v1/bl_NEW")
	lister := &countingCommentLister{comments: []github.PRComment{{Body: older}, {Body: newer}}}

	got, ok := newPRProvenanceReader(lister.list).Resolve(context.Background(), ref, 1)

	require.True(t, ok)
	assert.Equal(t, "bl_NEW", got.ItemID)
}
