package services

import (
	"context"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func browseReq(mut func(*sessionv1.ListSessionArtifactsRequest)) *connect.Request[sessionv1.ListSessionArtifactsRequest] {
	m := &sessionv1.ListSessionArtifactsRequest{}
	if mut != nil {
		mut(m)
	}
	return connect.NewRequest(m)
}

func browseFixture() *ArtifactService {
	at := func(d time.Duration) time.Time { return artifactT0.Add(d) }
	pr := func(o, r string, n int, d time.Duration) domain.PRRecord {
		p := prRecord(o, r, n, domain.PRSourceCreated)
		p.FirstSeenAt = at(d)
		return p
	}
	return newArtifactSvc(nil,
		artifactSession("u1", "one", 0, scannedBlob(
			pr("acme", "app", 12, 3*time.Hour),
			domain.CommitRecord{SHA: "1a2b3c4d5e6f1a2b3c4d5e6f1a2b3c4d5e6f1a2b", Subject: "fix", FirstSeenAt: at(2 * time.Hour)},
		)),
		artifactSession("u2", "two", time.Hour, scannedBlob(pr("acme", "other", 3, time.Hour))),
		artifactSession("u3", "three", 2*time.Hour, nil),
	)
}

func TestListSessionArtifacts_ShouldFilterByTypeAndRepo(t *testing.T) {
	t.Parallel()
	svc := browseFixture()
	before := ArtifactBrowseCount()
	resp, err := svc.ListSessionArtifacts(context.Background(), browseReq(func(r *sessionv1.ListSessionArtifactsRequest) {
		r.Types = []string{"pr"}
		r.Repo = "ACME/app"
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Rows, 1)
	row := resp.Msg.Rows[0]
	assert.Equal(t, "u1", row.SessionId)
	assert.Equal(t, int32(12), row.PullRequest.GetNumber())
	assert.Equal(t, "unknown", row.PullRequest.GetState(), "untracked PR gets a minimal entry")
	assert.Equal(t, "artifact", row.Source)
	assert.Equal(t, int32(1), resp.Msg.TotalCount)
	assert.Equal(t, int32(1), resp.Msg.UnscannedSessionCount, "u3 has no blob")
	assert.Greater(t, ArtifactBrowseCount(), before)
}

func TestListSessionArtifacts_ShouldReturnNewestFirstAndPage(t *testing.T) {
	t.Parallel()
	svc := browseFixture()
	first, err := svc.ListSessionArtifacts(context.Background(), browseReq(func(r *sessionv1.ListSessionArtifactsRequest) { r.PageSize = 2 }))
	require.NoError(t, err)
	require.Len(t, first.Msg.Rows, 2)
	assert.Equal(t, "pr", first.Msg.Rows[0].Record.Kind)
	assert.Equal(t, "commit", first.Msg.Rows[1].Record.Kind)
	assert.Equal(t, int32(3), first.Msg.TotalCount)
	require.NotEmpty(t, first.Msg.NextPageToken)

	second, err := svc.ListSessionArtifacts(context.Background(), browseReq(func(r *sessionv1.ListSessionArtifactsRequest) {
		r.PageSize = 2
		r.PageToken = first.Msg.NextPageToken
	}))
	require.NoError(t, err)
	require.Len(t, second.Msg.Rows, 1)
	assert.Equal(t, "u2", second.Msg.Rows[0].SessionId)
	assert.Empty(t, second.Msg.NextPageToken)
}

func TestListSessionArtifacts_ShouldRejectABadPageToken(t *testing.T) {
	t.Parallel()
	_, err := browseFixture().ListSessionArtifacts(context.Background(), browseReq(func(r *sessionv1.ListSessionArtifactsRequest) { r.PageToken = "!!" }))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestListSessionArtifacts_ShouldKeepOnlySessionsTiedToThePR(t *testing.T) {
	t.Parallel()
	svc := browseFixture()
	resp, err := svc.ListSessionArtifacts(context.Background(), browseReq(func(r *sessionv1.ListSessionArtifactsRequest) {
		r.Pr = &sessionv1.PullRequestRef{Owner: "acme", Repo: "other", Number: 3}
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Rows, 1)
	assert.Equal(t, "u2", resp.Msg.Rows[0].SessionId)
}

func TestListSessionArtifacts_ShouldUseTheSessionsOwnPRState(t *testing.T) {
	t.Parallel()
	ref, err := domain.NewPRRef("", "acme", "app", 12)
	require.NoError(t, err)
	inst := artifactSession("u1", "one", 0, scannedBlob(prRecord("acme", "app", 12, domain.PRSourceCreated)))
	require.NoError(t, inst.AttachPR(ref, domain.AttachSourceCreatedFromURL, domain.PRInitial{State: domain.PRStateMerged, Terminal: true, URL: "https://github.com/acme/app/pull/12"}))
	svc := newArtifactSvc(nil, inst)
	resp, err := svc.ListSessionArtifacts(context.Background(), browseReq(func(r *sessionv1.ListSessionArtifactsRequest) { r.State = "merged" }))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Rows, 1)
	assert.Equal(t, "session_pr", resp.Msg.Rows[0].Source)
}

func TestListSessionArtifacts_ShouldRedactLegacyBlobs(t *testing.T) {
	t.Parallel()
	svc := newArtifactSvc(nil, artifactSession("u1", "one", 0, scannedBlob(
		domain.CILinkRecord{URL: "https://ci.example.com/runs/9?token=abcd1234secret", FirstSeenAt: artifactT0},
	)))
	resp, err := svc.ListSessionArtifacts(context.Background(), browseReq(nil))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Rows, 1)
	assert.Equal(t, "https://ci.example.com/runs/9?token=[redacted]", resp.Msg.Rows[0].GetRecord().GetCiLink().GetUrl())
}
