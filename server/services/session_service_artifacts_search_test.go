package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/search"
)

type staticPRLinks map[string][]string // PRRef.String() -> session uuids

func (s staticPRLinks) SessionUUIDsByPR(_ context.Context, ref domain.PRRef) ([]string, error) {
	return s[ref.String()], nil
}

var artifactT0 = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

func scannedBlob(recs ...domain.ArtifactRecord) *domain.SessionArtifactsBlob {
	return &domain.SessionArtifactsBlob{Records: recs, ScanState: domain.ScanStateHasData}
}

func artifactSession(uuid, title string, age time.Duration, blob *domain.SessionArtifactsBlob) *session.Instance {
	return &session.Instance{UUID: uuid, Title: title, UpdatedAt: artifactT0.Add(age), Artifacts: blob}
}

// newArtifactSvc indexes every instance's blob, as the OnScanComplete wiring does.
func newArtifactSvc(links staticPRLinks, insts ...*session.Instance) *ArtifactService {
	idx := search.NewArtifactIndex(links)
	for _, in := range insts {
		idx.Update(in.UUID, in.Artifacts)
	}
	return &ArtifactService{index: idx, allInstances: func() []*session.Instance { return insts }}
}

func artifactSearchReq(q string) *connect.Request[sessionv1.SearchSessionArtifactsRequest] {
	return connect.NewRequest(&sessionv1.SearchSessionArtifactsRequest{Query: q})
}

func findByPRReq(owner, repo string, n int32) *connect.Request[sessionv1.FindSessionsByPullRequestRequest] {
	return connect.NewRequest(&sessionv1.FindSessionsByPullRequestRequest{
		Pr: &sessionv1.PullRequestRef{Owner: owner, Repo: repo, Number: n},
	})
}

func hitIDs(hits []*sessionv1.SessionArtifactHit) []string {
	ids := make([]string, 0, len(hits))
	for _, h := range hits {
		ids = append(ids, h.SessionId)
	}
	return ids
}

func prRecord(owner, repo string, n int, src domain.PRSource) domain.PRRecord {
	return domain.PRRecord{Host: "github.com", Owner: owner, Repo: repo, Number: n, Source: src}
}

func TestSearchSessionArtifacts_ShouldReturnTheOwningSession_ForEachKindOfString(t *testing.T) {
	t.Parallel()
	svc := newArtifactSvc(nil,
		artifactSession("u1", "one", 0, scannedBlob(
			prRecord("acme", "app", 77, domain.PRSourceCreated),
			domain.BranchRecord{Name: "feat/zebra-stripes", Action: "pushed"},
			domain.CommitRecord{SHA: "1a2b3c4d5e6f1a2b3c4d5e6f1a2b3c4d5e6f1a2b", Subject: "stripe"},
			domain.PlanDocRecord{Path: "project_plans/zebra/plan.md"},
		)),
		artifactSession("u2", "two", time.Hour, scannedBlob(domain.BranchRecord{Name: "other"})),
	)
	tests := []struct {
		name, query, kind string
		wantType          string
	}{
		{"pr url", "https://github.com/acme/app/pull/77", "pr", "pr"},
		{"pr ref", "acme/app#77", "pr", "pr"},
		{"branch", "feat/zebra-stripes", "branch", "branch"},
		{"sha prefix", "1a2b3c4", "sha", "commit"},
		{"long sha prefix", "1a2b3c4d5e", "sha", "commit"},
		{"plan doc path", "project_plans/zebra/plan.md", "branch", "plan_doc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			before := ArtifactSearchQueryCount(tt.kind, "hit")
			resp, err := svc.SearchSessionArtifacts(context.Background(), artifactSearchReq(tt.query))
			require.NoError(t, err)
			require.NotEmpty(t, resp.Msg.Hits, "query %q", tt.query)
			assert.Equal(t, "u1", resp.Msg.Hits[0].SessionId)
			assert.Equal(t, tt.wantType, resp.Msg.Hits[0].Matches[0].Type)
			assert.Greater(t, ArtifactSearchQueryCount(tt.kind, "hit"), before)
		})
	}
}

func TestSearchSessionArtifacts_ShouldCountAMiss_AndAcceptTwoCharsButNotOne(t *testing.T) {
	t.Parallel()
	svc := newArtifactSvc(nil, artifactSession("u1", "one", 0, scannedBlob(domain.BranchRecord{Name: "feat-x"})))
	before := ArtifactSearchQueryCount("text", "miss")
	resp, err := svc.SearchSessionArtifacts(context.Background(), artifactSearchReq("nonexistentzzz"))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Hits)
	assert.Greater(t, ArtifactSearchQueryCount("text", "miss"), before)

	_, err = svc.SearchSessionArtifacts(context.Background(), artifactSearchReq("ab"))
	require.NoError(t, err, "2 characters run the search")
	for _, q := range []string{"a", "", " a "} {
		_, err = svc.SearchSessionArtifacts(context.Background(), artifactSearchReq(q))
		require.Error(t, err, "query %q", q)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "query %q", q)
	}
}

func TestSearchSessionArtifacts_ShouldFollowHiddenAndArchivedVisibility(t *testing.T) {
	t.Parallel()
	blob := func() *domain.SessionArtifactsBlob {
		return scannedBlob(domain.BranchRecord{Name: "shared-branch"})
	}
	hidden := artifactSession("uh", "hidden", 0, blob())
	hidden.Hidden = true
	archived := artifactSession("ua", "archived", 0, blob())
	at := artifactT0
	archived.ArchivedAt = &at
	svc := newArtifactSvc(nil, artifactSession("uv", "visible", 0, blob()), hidden, archived)

	ids := func(excludeAuto, inclArchived bool) []string {
		resp, err := svc.SearchSessionArtifacts(context.Background(), connect.NewRequest(&sessionv1.SearchSessionArtifactsRequest{
			Query: "shared-branch", ExcludeAutomationSessions: excludeAuto, IncludeArchived: inclArchived,
		}))
		require.NoError(t, err)
		return hitIDs(resp.Msg.Hits)
	}
	assert.ElementsMatch(t, []string{"uv", "uh"}, ids(false, false), "hidden kept unless excluded; archived dropped by default")
	assert.ElementsMatch(t, []string{"uv"}, ids(true, false))
	assert.ElementsMatch(t, []string{"uv", "uh", "ua"}, ids(false, true))
}

func TestSearchSessionArtifacts_ShouldRestrictToThePRFilter(t *testing.T) {
	t.Parallel()
	svc := newArtifactSvc(staticPRLinks{"github.com/acme/app#12": {"u3"}},
		artifactSession("u1", "one", 0, scannedBlob(prRecord("acme", "app", 12, domain.PRSourceCreated), domain.BranchRecord{Name: "common-branch"})),
		artifactSession("u2", "two", 0, scannedBlob(domain.BranchRecord{Name: "common-branch"})),
		artifactSession("u3", "three", 0, scannedBlob(domain.BranchRecord{Name: "common-branch"})),
	)
	resp, err := svc.SearchSessionArtifacts(context.Background(), connect.NewRequest(&sessionv1.SearchSessionArtifactsRequest{
		Query: "common-branch", Pr: &sessionv1.PullRequestRef{Owner: "acme", Repo: "app", Number: 12},
	}))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"u1", "u3"}, hitIDs(resp.Msg.Hits))
}

func TestUnscannedCount_ShouldCountNotLinkedAndScanningVisibleSessions(t *testing.T) {
	t.Parallel()
	var insts []*session.Instance
	insts = append(insts, artifactSession("s0", "s0", 0, scannedBlob(domain.BranchRecord{Name: "x-branch"})))
	for i := 1; i < 7; i++ {
		insts = append(insts, artifactSession("s"+string(rune('0'+i)), "s"+string(rune('0'+i)), 0, scannedBlob()))
	}
	insts = append(insts,
		artifactSession("n1", "n1", 0, nil), // no blob yet
		artifactSession("n2", "n2", 0, &domain.SessionArtifactsBlob{ScanState: domain.ScanStateNotLinked}),
		artifactSession("n3", "n3", 0, &domain.SessionArtifactsBlob{ScanState: domain.ScanStateScanning}),
	)
	hiddenUnscanned := artifactSession("n4", "n4", 0, nil)
	hiddenUnscanned.Hidden = true
	insts = append(insts, hiddenUnscanned)
	svc := newArtifactSvc(nil, insts...)

	resp, err := svc.SearchSessionArtifacts(context.Background(), connect.NewRequest(&sessionv1.SearchSessionArtifactsRequest{
		Query: "x-branch", ExcludeAutomationSessions: true,
	}))
	require.NoError(t, err)
	assert.EqualValues(t, 3, resp.Msg.UnscannedSessionCount, "10 visible sessions, 3 not scanned; the hidden one is not visible")

	found, err := svc.FindSessionsByPullRequest(context.Background(), connect.NewRequest(&sessionv1.FindSessionsByPullRequestRequest{
		Pr: &sessionv1.PullRequestRef{Owner: "acme", Repo: "app", Number: 1}, ExcludeAutomationSessions: true,
	}))
	require.NoError(t, err)
	assert.EqualValues(t, 3, found.Msg.UnscannedSessionCount)
}

func TestFindSessionsByPullRequest_ShouldFilterOrphans_AndNeverSeedFromThem(t *testing.T) {
	t.Parallel()
	svc := newArtifactSvc(staticPRLinks{"github.com/acme/app#12": {"u1", "deleted-uuid"}},
		artifactSession("u1", "one", 0, scannedBlob()))
	resp, err := svc.FindSessionsByPullRequest(context.Background(), findByPRReq("acme", "app", 12))
	require.NoError(t, err)
	assert.Equal(t, []string{"u1"}, hitIDs(resp.Msg.Hits))
	assert.Equal(t, "session_pr", resp.Msg.Hits[0].Source)
}

// U-151: Tracked first, then Seen in transcript; each group by last_activity
// descending with the title ascending as the tie-break; a session in both is one
// Tracked row.
func TestFindSessionsByPullRequest_ShouldOrderTrackedFirstThenByActivityThenTitle_U151(t *testing.T) {
	t.Parallel()
	seen := scannedBlob(prRecord("acme", "app", 12, domain.PRSourceMentioned))
	svc := newArtifactSvc(staticPRLinks{"github.com/acme/app#12": {"t-old", "t-new", "both"}},
		artifactSession("t-old", "tracked old", -3*time.Hour, scannedBlob()),
		artifactSession("t-new", "tracked new", -time.Hour, scannedBlob()),
		artifactSession("both", "both sources", -2*time.Hour, seen),
		artifactSession("s-b", "b seen", -time.Hour, seen),
		artifactSession("s-a", "a seen", -time.Hour, seen),
		artifactSession("s-newest", "z seen newest", 0, seen),
	)
	resp, err := svc.FindSessionsByPullRequest(context.Background(), findByPRReq("acme", "app", 12))
	require.NoError(t, err)
	assert.Equal(t, []string{"t-new", "both", "t-old", "s-newest", "s-a", "s-b"}, hitIDs(resp.Msg.Hits))
	for i, want := range []string{"session_pr", "session_pr", "session_pr", "artifact", "artifact", "artifact"} {
		assert.Equal(t, want, resp.Msg.Hits[i].Source, "row %d", i)
	}
}

func TestFindSessionsByPullRequest_ShouldTreatCaseVariantsAsOnePR(t *testing.T) {
	t.Parallel()
	svc := newArtifactSvc(staticPRLinks{"github.com/acme/app#12": {"u1"}},
		artifactSession("u1", "one", 0, scannedBlob()),
		artifactSession("u2", "two", 0, scannedBlob(prRecord("Acme", "App", 12, domain.PRSourceCreated))),
	)
	upper, err := svc.FindSessionsByPullRequest(context.Background(), findByPRReq("Acme", "App", 12))
	require.NoError(t, err)
	lower, err := svc.FindSessionsByPullRequest(context.Background(), findByPRReq("acme", "app", 12))
	require.NoError(t, err)
	assert.Equal(t, hitIDs(lower.Msg.Hits), hitIDs(upper.Msg.Hits))
	assert.Equal(t, []string{"u1", "u2"}, hitIDs(upper.Msg.Hits))
}

func TestFindSessionsByPullRequest_ShouldRejectAnUnsetPR(t *testing.T) {
	t.Parallel()
	svc := newArtifactSvc(nil)
	_, err := svc.FindSessionsByPullRequest(context.Background(), connect.NewRequest(&sessionv1.FindSessionsByPullRequestRequest{}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestArtifactSearch_ShouldFailPrecondition_WhenIndexNotWired(t *testing.T) {
	t.Parallel()
	svc := &ArtifactService{}
	_, err := svc.SearchSessionArtifacts(context.Background(), artifactSearchReq("abc"))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	_, err = svc.FindSessionsByPullRequest(context.Background(), findByPRReq("a", "b", 1))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestSearchSessionArtifacts_ShouldRejectOversizedQueries(t *testing.T) {
	t.Parallel()
	svc := newArtifactSvc(nil)
	for name, q := range map[string]string{
		"too long":       strings.Repeat("a", maxArtifactQueryLen+1),
		"too many terms": strings.TrimSpace(strings.Repeat("ab ", maxArtifactQueryTerms+1)),
	} {
		_, err := svc.SearchSessionArtifacts(context.Background(), artifactSearchReq(q))
		require.Error(t, err, name)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), name)
	}
	_, err := svc.SearchSessionArtifacts(context.Background(), artifactSearchReq(strings.TrimSpace(strings.Repeat("ab ", maxArtifactQueryTerms))))
	assert.NoError(t, err, "exactly the term cap is accepted")
}
