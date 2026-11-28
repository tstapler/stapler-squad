package services

// Differential golden for the GitHub PR RPCs, the user-PR panel and the Up Next
// nudge, recorded from origin/main before the move.

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/prs"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

var updateGitHubGolden = flag.Bool("update-github-golden", false, "rewrite testdata/github.golden")

type goldenOut struct {
	t *testing.T
	b strings.Builder
}

func (o *goldenOut) line(format string, args ...any) { fmt.Fprintf(&o.b, format+"\n", args...) }

// record takes the message only when err is nil; pass the response's Msg through msgOf.
func (o *goldenOut) record(step string, msg proto.Message, err error) {
	if err != nil {
		o.line("%s err code=%v %v", step, connect.CodeOf(err), err)
		return
	}
	raw, merr := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(msg)
	require.NoError(o.t, merr)
	var v any
	require.NoError(o.t, json.Unmarshal(raw, &v))
	norm, merr := json.Marshal(v)
	require.NoError(o.t, merr)
	o.line("%s %s", step, norm)
}

func msgOf[T any, P interface {
	*T
	proto.Message
}](r *connect.Response[T]) proto.Message {
	if r == nil {
		return nil
	}
	return P(r.Msg)
}

func (o *goldenOut) logs(l *logCapture) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, ln := range l.lines {
		keys := make([]string, 0, len(ln.fields))
		for k := range ln.fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%v", k, ln.fields[k]))
		}
		o.line("  log %s %s", ln.msg, strings.Join(parts, " "))
	}
	l.lines = nil
}

func goldenNudgeCases(t *testing.T, o *goldenOut) {
	cases := []struct {
		name  string
		setup func(f *nudgeFixture)
		sid   string
	}{
		{"delivered", func(*nudgeFixture) {}, "fix-ci"},
		{"unlinked", func(*nudgeFixture) {}, "other"},
		{"unknown-pr", func(f *nudgeFixture) { f.prs.prs = nil }, "fix-ci"},
		{"not-running", func(f *nudgeFixture) { f.nudger.insts = nil }, "fix-ci"},
		{"title-mismatch", func(f *nudgeFixture) { f.inst.Title = "renamed"; f.inst.UUID = "fix-ci" }, "fix-ci"},
		{"hidden", func(f *nudgeFixture) { f.inst.Hidden = true }, "fix-ci"},
		{"paused", func(f *nudgeFixture) { f.inst.Status = domain.Paused }, "fix-ci"},
		{"program", func(f *nudgeFixture) { f.inst.Program = "aider" }, "fix-ci"},
		{"no-token", func(f *nudgeFixture) { f.prs.tokens = nil }, "fix-ci"},
		{"nothing", func(f *nudgeFixture) { f.fetcher.detail.FailingChecks = nil }, "fix-ci"},
		{"fetch-not-found", func(f *nudgeFixture) { f.fetcher.err = githubpkg.ErrGitHubRefNotFound }, "fix-ci"},
		{"fetch-rate-limited", func(f *nudgeFixture) { f.fetcher.err = githubpkg.ErrRateLimited }, "fix-ci"},
		{"fetch-rate-limited-until", func(f *nudgeFixture) {
			f.fetcher.err = githubpkg.ErrRateLimited
			f.svc.nudge.rateLimitResume = func() (bool, time.Time) { return true, time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC) }
		}, "fix-ci"},
		{"fetch-deadline", func(f *nudgeFixture) { f.fetcher.err = context.DeadlineExceeded }, "fix-ci"},
		{"fetch-other", func(f *nudgeFixture) { f.fetcher.err = errors.New("boom") }, "fix-ci"},
		{"attention", func(f *nudgeFixture) { f.prs.firstSeen = f.clock.Now().Add(-90 * time.Second) }, "fix-ci"},
		{"all-reasons", func(f *nudgeFixture) {
			conflict := true
			f.fetcher.detail.HasMergeConflict = &conflict
			f.fetcher.detail.UnresolvedThreads = []githubpkg.PRThreadRef{{}}
		}, "fix-ci"},
		{"unwired", func(f *nudgeFixture) { f.svc.nudge.fetcher = nil }, "fix-ci"},
	}
	for _, c := range cases {
		f := newNudgeFixture(t)
		f.svc.nudge.rateLimitResume = func() (bool, time.Time) { return false, time.Time{} }
		rec := &fakeNudgeRecorder{}
		f.svc.SetNudgeRecorder(rec)
		c.setup(f)
		resp, err := f.call(c.sid)
		o.record("nudge/"+c.name, resp, err)
		for _, s := range f.nudger.steers {
			o.line("  steer sig=%s msg=%q", s.sig, s.msg)
		}
		o.line("  fetches=%d finds=%d manual=%v tracked=%v", f.fetcher.callCount(), f.nudger.finds, rec.calls, f.prs.recorded)
		o.logs(f.logs)
	}
	for _, outcome := range []access.SteerOutcome{access.SteerUnspecified, access.SteerDelivered, access.SteerGuardBusy, access.SteerDuplicate, access.SteerCoolingDown,
		access.SteerBusy, access.SteerNoStatusSource, access.SteerNotTracked, access.SteerFailed, access.SteerOutcome(99)} {
		for _, serr := range []error{nil, errors.New("write boom"), fmt.Errorf("x: %w", access.ErrSteerPaneOwnership)} {
			f := newNudgeFixture(t)
			rec := &fakeNudgeRecorder{}
			f.svc.SetNudgeRecorder(rec)
			f.nudger.outcome, f.nudger.err = outcome, serr
			resp, err := f.call("fix-ci")
			o.record(fmt.Sprintf("steer/%s/%v", outcome, serr), resp, err)
			o.line("  manual=%v tracked=%v", rec.calls, f.prs.recorded)
			o.logs(f.logs)
		}
	}
	for _, bad := range []*sessionv1.PRKey{nil, {Owner: "", Repo: "r", Number: 1}, {Owner: "o", Repo: "r"}} {
		f := newNudgeFixture(t)
		r, err := f.svc.NudgeSessionForPR(context.Background(), connect.NewRequest(&sessionv1.NudgeSessionForPRRequest{Pr: bad}))
		o.record("nudge/bad-key", msgOf(r), err)
		o.logs(f.logs)
	}
}

func goldenPanelCases(t *testing.T, o *goldenOut) {
	f := newNudgeFixture(t)
	n := 3
	conflict := true
	at := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	full := []githubpkg.UserPR{{
		Owner: "acme", Repo: "api", Number: 42, Title: "t", URL: "u", State: "open", HeadRef: "h", BaseRef: "b",
		CheckConclusion: "failure", ApprovedCount: 1, ChangesReqCount: 2, SessionIDs: []string{"s"},
		LocalWorktreePath: "/w", FailingChecks: []githubpkg.FailingCheck{{Name: "lint", URL: "x", Conclusion: "failure"}},
		HasMergeConflict: &conflict, DetailsLoaded: true, AccountLogin: "alice", UnresolvedThreadCount: &n,
		UnresolvedThreadsTruncated: true, NodeID: "N", Body: "body", Labels: []string{"l"}, Additions: 1, Deletions: 2,
		ChangedFiles: 3, RequestedReviewerLogins: []string{"bob"}, UpdatedAt: at, ClosedAt: at, MergedAt: at,
		LinkedSessions: []githubpkg.LinkedSession{
			{SessionID: "fix-ci", Status: githubpkg.LinkedSessionRunning, LastActiveAt: at},
			{SessionID: "gone", Status: githubpkg.LinkedSessionPaused},
			{SessionID: "legacy", Status: githubpkg.LinkedSessionStopped, LegacyFallback: true},
			{SessionID: "unknown"},
		},
	}, {Owner: "o", Repo: "r", Number: 1, IsDraft: true}}
	protoPRs := f.svc.userPRsToProto(full)
	o.record("panel/prs", &sessionv1.ListUserPRsResponse{Prs: protoPRs, AccountStatuses: accountStatusesToProto([]githubpkg.AccountPollStatus{
		{AccountLogin: "a", State: githubpkg.AccountPollOK},
		{Host: "ghe", AccountLogin: "b", State: githubpkg.AccountPollUnauthorized, Detail: "d"},
		{AccountLogin: "c", State: githubpkg.AccountPollRateLimited},
		{AccountLogin: "d", State: githubpkg.AccountPollError},
	})}, nil)
	f.svc.logUpNextFunnel(protoPRs)
	f.svc.logUpNextFunnel(protoPRs)
	o.logs(f.logs)

	for _, c := range []struct {
		name   string
		tokens bool
		err    error
		cached bool
	}{
		{"ok", true, nil, true},
		{"fail", true, errors.New("nope"), true},
		{"rate", true, rateLimitedUntilErr(time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC)), true},
		{"no-account", false, nil, true},
		{"no-node", true, nil, false},
	} {
		r := &actionRecorder{err: c.err}
		s := newActionService(r, c.tokens)
		if !c.cached {
			s.actions.snapshot = fixedSnapshot{}
		}
		ctx := context.Background()
		m1, err := s.MergePRDirect(ctx, connect.NewRequest(&sessionv1.MergePRDirectRequest{Owner: "o", Repo: "r", Number: 7}))
		o.record("action/merge/"+c.name, msgOf(m1), err)
		m2, err := s.ClosePRDirect(ctx, connect.NewRequest(&sessionv1.ClosePRDirectRequest{Owner: "o", Repo: "r", Number: 7}))
		o.record("action/close/"+c.name, msgOf(m2), err)
		m3, err := s.ConvertPRToDraft(ctx, connect.NewRequest(&sessionv1.PRDraftToggleRequest{Owner: "o", Repo: "r", Number: 7}))
		o.record("action/draft/"+c.name, msgOf(m3), err)
		m4, err := s.MarkPRReady(ctx, connect.NewRequest(&sessionv1.PRDraftToggleRequest{Owner: "o", Repo: "r", Number: 7}))
		o.record("action/ready/"+c.name, msgOf(m4), err)
		o.line("  merged=%d closed=%d refreshed=%d method=%q", r.merged, r.closed, r.refreshed, r.method)
	}
}

func goldenSessionPRCases(t *testing.T, o *goldenOut) {
	ctx := context.Background()
	for _, n := range []int{0, 2} {
		inst := &instance.Instance{Title: "multi", UUID: "uuid-multi", GitHubOwner: "acme", GitHubRepo: "app"}
		if n > 0 {
			for _, num := range []int{11, 12} {
				ref, err := domain.NewPRRef("", "acme", "app", num)
				require.NoError(t, err)
				require.NoError(t, inst.AttachPR(ref, domain.AttachSourceBranchDiscovery, domain.PRInitial{State: domain.PRStateOpen}))
			}
		}
		rec := &recordingPRActions{}
		inst.SetPRActionClient(rec)
		svc := NewGitHubService(nil)
		svc.find = func(string) (*instance.Instance, error) { return inst, nil }
		for _, sel := range []*sessionv1.PullRequestRef{nil, protoRef(11), protoRef(99), {Owner: "acme", Repo: "app"}} {
			step := fmt.Sprintf("session/%d/%d", n, sel.GetNumber())
			c, err := svc.GetPRComments(ctx, connect.NewRequest(&sessionv1.GetPRCommentsRequest{Id: "multi", Pr: sel}))
			o.record(step+"/comments", msgOf(c), err)
			p, err := svc.PostPRComment(ctx, connect.NewRequest(&sessionv1.PostPRCommentRequest{Id: "multi", Body: "b", Pr: sel}))
			o.record(step+"/post", msgOf(p), err)
			m, err := svc.MergePR(ctx, connect.NewRequest(&sessionv1.MergePRRequest{Id: "multi", Pr: sel, MergeMethod: sessionv1.MergeMethod_MERGE_METHOD_SQUASH}))
			o.record(step+"/merge", msgOf(m), err)
			cl, err := svc.ClosePR(ctx, connect.NewRequest(&sessionv1.ClosePRRequest{Id: "multi", Pr: sel}))
			o.record(step+"/close", msgOf(cl), err)
		}
		info, err := svc.GetPRInfo(ctx, connect.NewRequest(&sessionv1.GetPRInfoRequest{}))
		o.record(fmt.Sprintf("session/%d/info-empty", n), msgOf(info), err)
		o.line("  calls=%v", rec.calls)
	}
	for _, at := range []time.Time{time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC)} {
		o.line("classify %v", prs.ClassifyGitHubRateLimitError(rateLimitedUntilErr(at)))
	}
}

func TestGitHubGolden(t *testing.T) {
	o := &goldenOut{t: t}
	goldenNudgeCases(t, o)
	goldenPanelCases(t, o)
	goldenSessionPRCases(t, o)

	const path = "testdata/github.golden"
	if *updateGitHubGolden {
		require.NoError(t, os.WriteFile(path, []byte(o.b.String()), 0o600))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), o.b.String())
}
