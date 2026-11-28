package services

// Differential golden for the artifact RPCs, the ListSessions PR selector and
// GetVCSStatus, recorded through the hub's public surface before the move.

import (
	"cmp"
	"context"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/search"
)

var updateArtifactVCSGolden = flag.Bool("update-artifact-vcs-golden", false, "rewrite testdata/artifact_vcs_hub.golden")

type goldenPRLinks map[string][]string

func (l goldenPRLinks) SessionUUIDsByPR(_ context.Context, ref domain.PRRef) ([]string, error) {
	return l[ref.String()], nil
}

type goldenRescanner struct{ files []string }

func (r *goldenRescanner) RescanSession(f string) { r.files = append(r.files, f) }

func goldenArtifactInstances() []*instance.Instance {
	t0 := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	at := func(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }
	pr := func(owner, repo string, n, m int) domain.PRRecord {
		return domain.PRRecord{Host: "github.com", Owner: owner, Repo: repo, Number: n, Source: domain.PRSourceCreated, FirstSeenAt: at(m)}
	}
	mk := func(uuid, title string, age int, state domain.ScanState, recs ...domain.ArtifactRecord) *instance.Instance {
		in := &instance.Instance{UUID: uuid, Title: title, UpdatedAt: at(age), Status: domain.Paused}
		if state != "" {
			in.Artifacts = &domain.SessionArtifactsBlob{Records: recs, ScanState: state}
		}
		return in
	}
	a := mk("u-a", "alpha", 1, domain.ScanStateHasData,
		pr("acme", "app", 7, 5),
		domain.BranchRecord{Name: "feat/zebra", Action: "pushed", FirstSeenAt: at(4)},
		domain.CommitRecord{SHA: "1a2b3c4d5e6f1a2b3c4d5e6f1a2b3c4d5e6f1a2b", Subject: "stripe", FirstSeenAt: at(3)},
		domain.PlanDocRecord{Path: "project_plans/zebra/plan.md", FirstSeenAt: at(2)},
	)
	a.SetHistoryInfo("conv", "/nonexistent/history.jsonl")
	b := mk("u-b", "beta", 9, domain.ScanStateHasData, pr("Acme", "App", 7, 8), pr("other", "lib", 3, 7))
	hidden := mk("u-h", "hidden", 3, domain.ScanStateHasData, domain.BranchRecord{Name: "feat/zebra", FirstSeenAt: at(1)})
	hidden.Hidden = true
	archived := mk("u-x", "archived", 2, domain.ScanStateHasData, pr("acme", "app", 7, 1))
	when := at(2)
	archived.ArchivedAt = &when
	return []*instance.Instance{a, b, hidden, archived,
		mk("u-n", "noblob", 0, ""), mk("u-s", "scanning", 0, domain.ScanStateScanning)}
}

// goldenSortTies orders equal-score matches: the index returns them in map order.
func goldenSortTies(hits []*sessionv1.SessionArtifactHit) {
	for _, h := range hits {
		slices.SortStableFunc(h.Matches, func(a, b *sessionv1.ArtifactMatch) int {
			return cmp.Or(cmp.Compare(b.Score, a.Score), cmp.Compare(a.Type, b.Type), cmp.Compare(a.Text, b.Text))
		})
	}
}

func goldenArtifactRPCs(t *testing.T, o *goldenOut) {
	ctx := context.Background()
	svc := NewSessionService(svctest.CreateTestStorage(t), nil)
	poller := svctest.NewEmptyPoller()
	poller.SetInstances(goldenArtifactInstances())
	svc.ReviewQueuePoller = poller

	search1 := func(name string, req *sessionv1.SearchSessionArtifactsRequest) {
		resp, err := svc.SearchSessionArtifacts(ctx, connect.NewRequest(req))
		if err == nil {
			goldenSortTies(resp.Msg.Hits)
		}
		o.record("search/"+name, msgOf(resp), err)
	}
	find := func(name string, req *sessionv1.FindSessionsByPullRequestRequest) {
		resp, err := svc.FindSessionsByPullRequest(ctx, connect.NewRequest(req))
		o.record("find/"+name, msgOf(resp), err)
	}
	list := func(name string, req *sessionv1.ListSessionArtifactsRequest) string {
		resp, err := svc.ListSessionArtifacts(ctx, connect.NewRequest(req))
		o.record("list/"+name, msgOf(resp), err)
		if err != nil {
			return ""
		}
		return resp.Msg.NextPageToken
	}
	rescan := func(name, id string) {
		resp, err := svc.RescanSessionArtifacts(ctx, connect.NewRequest(&sessionv1.RescanSessionArtifactsRequest{Id: id}))
		o.record("rescan/"+name, msgOf(resp), err)
	}
	ref := &sessionv1.PullRequestRef{Owner: "acme", Repo: "app", Number: 7}

	search1("no-index", &sessionv1.SearchSessionArtifactsRequest{Query: "zebra"})
	find("no-index", &sessionv1.FindSessionsByPullRequestRequest{Pr: ref})
	list("no-index-pr", &sessionv1.ListSessionArtifactsRequest{Pr: ref})
	list("no-index", &sessionv1.ListSessionArtifactsRequest{})
	rescan("no-rescanner", "alpha")

	rescanner := &goldenRescanner{}
	svc.SetArtifactRescanner(rescanner)
	rescan("empty", "")
	rescan("unknown", "nope")
	rescan("no-history", "beta")
	rescan("history", "alpha")
	o.line("rescanned=%v", rescanner.files)

	prRef, err := domain.NewPRRef("", "acme", "app", 7)
	require.NoError(t, err)
	idx := search.NewArtifactIndex(goldenPRLinks{prRef.String(): {"u-a", "u-gone"}})
	for _, in := range goldenArtifactInstances() {
		idx.Update(in.UUID, in.Artifacts)
	}
	svc.SetArtifactIndex(idx)

	for _, q := range []string{"a", "zebra", "feat/zebra", "1a2b3c4", "acme/app#7", "https://github.com/acme/app/pull/7",
		"nothingzzz", strings.Repeat("a", 257), strings.Repeat("ab ", 17)} {
		search1(q[:min(len(q), 40)], &sessionv1.SearchSessionArtifactsRequest{Query: q})
	}
	search1("exclude-automation", &sessionv1.SearchSessionArtifactsRequest{Query: "feat/zebra", ExcludeAutomationSessions: true})
	search1("archived", &sessionv1.SearchSessionArtifactsRequest{Query: "acme/app#7", IncludeArchived: true})
	search1("limit", &sessionv1.SearchSessionArtifactsRequest{Query: "acme/app#7", Limit: 1})
	search1("pr-filter", &sessionv1.SearchSessionArtifactsRequest{Query: "zebra", Pr: ref})
	search1("pr-invalid", &sessionv1.SearchSessionArtifactsRequest{Query: "zebra", Pr: &sessionv1.PullRequestRef{}})
	find("unset", &sessionv1.FindSessionsByPullRequestRequest{})
	find("bad", &sessionv1.FindSessionsByPullRequestRequest{Pr: &sessionv1.PullRequestRef{Number: 1}})
	find("ok", &sessionv1.FindSessionsByPullRequestRequest{Pr: ref, IncludeArchived: true})

	list("all", &sessionv1.ListSessionArtifactsRequest{IncludeArchived: true})
	list("types", &sessionv1.ListSessionArtifactsRequest{Types: []string{"pr", "commit"}})
	list("repo", &sessionv1.ListSessionArtifactsRequest{Repo: " ACME/app "})
	list("state", &sessionv1.ListSessionArtifactsRequest{State: "Unknown"})
	list("pr", &sessionv1.ListSessionArtifactsRequest{Pr: ref})
	list("pr-invalid", &sessionv1.ListSessionArtifactsRequest{Pr: &sessionv1.PullRequestRef{Owner: "acme"}})
	list("automation", &sessionv1.ListSessionArtifactsRequest{ExcludeAutomationSessions: true})
	list("bad-token", &sessionv1.ListSessionArtifactsRequest{PageToken: "!!"})
	list("past-end", &sessionv1.ListSessionArtifactsRequest{PageToken: "OTk"})
	for token, page := "", 0; page < 5; page++ {
		token = list("page", &sessionv1.ListSessionArtifactsRequest{PageSize: 3, PageToken: token})
		if token == "" {
			break
		}
	}

	for i, sel := range []*sessionv1.PullRequestRef{{}, {Number: 2}} {
		_, err := svc.ListSessions(ctx, connect.NewRequest(&sessionv1.ListSessionsRequest{Pr: sel}))
		o.line("sessions/%d code=%v %v", i, connect.CodeOf(err), err)
	}
}

var goldenVolatile = regexp.MustCompile(`"(statusAsOf|authoredAt|sha|headCommit|shortSha|path|repoPath)":"[^"]*"`)

func goldenVCSStatus(t *testing.T, o *goldenOut) {
	ctx := context.Background()
	dir, baseSHA, _ := svctest.NewVCSStatusTestRepo(t)
	// Absolute and nonexistent: a temp dir can sit under somebody's repository (GOTMPDIR
	// below a jj repo in $HOME), and the VCS lookup walks the parents.
	const plain = "/ssq-golden-no-vcs/plain"
	// out records a response with the repo's own paths, hashes and times blanked.
	out := func(name string, ws *WorkspaceService, req *sessionv1.GetVCSStatusRequest) {
		sub := &goldenOut{t: t}
		resp, err := ws.GetVCSStatus(ctx, connect.NewRequest(req))
		sub.record("vcs/"+name, msgOf(resp), err)
		line := strings.ReplaceAll(sub.b.String(), plain, "<plain>")
		o.b.WriteString(goldenVolatile.ReplaceAllString(line, `"$1":"*"`))
	}
	service := func(inst *instance.Instance) *WorkspaceService {
		ws := NewWorkspaceService(nil, nil)
		ws.SetLiveFinder(&svctest.StubLiveFinder{Inst: inst})
		return ws
	}
	mk := func(path, base string) *instance.Instance {
		inst := &instance.Instance{Title: "golden", Path: path, Status: domain.Paused, Program: "claude"}
		if base != "" {
			inst.SetDirBaseSHA(base)
		}
		return inst
	}
	req := &sessionv1.GetVCSStatusRequest{Id: "golden"}
	fresh := &sessionv1.GetVCSStatusRequest{Id: "golden", WaitForFresh: true}

	out("empty-id", service(mk(dir, "")), &sessionv1.GetVCSStatusRequest{})
	out("unknown", NewWorkspaceService(nil, nil), req)
	out("no-workdir", service(mk("", "")), req)
	out("not-vcs", service(mk(plain, "")), req)
	out("no-base", service(mk(dir, "")), req)
	out("bad-base", service(mk(dir, "0000000000000000000000000000000000dead")), req)
	ws := service(mk(dir, baseSHA))
	out("miss", ws, req)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x\n"), 0o600))
	out("hit", ws, req)
	out("hit-wait-for-fresh", ws, fresh)
	ws.InvalidateVCSStatus(dir)
	out("after-invalidate", ws, req)
}

func TestArtifactVCSHubGolden(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	o := &goldenOut{t: t}
	goldenArtifactRPCs(t, o)
	goldenVCSStatus(t, o)

	const path = "testdata/artifact_vcs_hub.golden"
	if *updateArtifactVCSGolden {
		require.NoError(t, os.WriteFile(path, []byte(o.b.String()), 0o600))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), o.b.String())
}
