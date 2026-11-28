package services

// Differential golden for the hub's side of the GitHub nudge cut: ask-to-fix, and
// Up Next's nudge through the real SessionService guard. Recorded before the move.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

var updateGitHubHubGolden = flag.Bool("update-github-hub-golden", false, "rewrite testdata/github_hub.golden")

func TestGitHubHubGolden(t *testing.T) {
	o := &goldenOut{t: t}
	ask := func(name string, status domain.Status, mutate func(f *askFixFixture), sel *sessionv1.PullRequestRef, opts ...func(*instance.Instance)) {
		f := newAskFixFixture(t, status, opts...)
		if mutate != nil {
			mutate(f)
		}
		for i := 0; i < 2; i++ {
			resp, err := f.ask(sel)
			o.record(fmt.Sprintf("ask/%s/%d", name, i), resp, err)
		}
		o.line("  order=%v fetches=%d", f.rec.order, f.fetcher.callCount())
	}
	ask("delivered", domain.Active, nil, protoRef(12))
	ask("not-attached", domain.Active, nil, protoRef(99))
	ask("no-token", domain.Active, nil, protoRef(11))
	ask("paused", domain.Paused, nil, protoRef(12))
	ask("clean", domain.Active, func(f *askFixFixture) { f.fetcher.detail.FailingChecks = nil }, protoRef(12))
	ask("hidden", domain.Active, nil, protoRef(12), func(i *instance.Instance) { i.Hidden = true })
	ask("program", domain.Active, nil, protoRef(12), func(i *instance.Instance) { i.Program = "aider" })
	ask("fetch-missing", domain.Active, func(f *askFixFixture) { f.fetcher.err = github.ErrGitHubRefNotFound }, protoRef(12))
	ask("fetch-other", domain.Active, func(f *askFixFixture) { f.fetcher.err = errors.New("boom") }, protoRef(12))
	ask("verify-fails", domain.Active, func(f *askFixFixture) { f.rec.verifyErr = errors.New("pane gone") }, protoRef(12))
	ask("write-fails", domain.Active, func(f *askFixFixture) { f.rec.writeErr = errors.New("write boom") }, protoRef(12))
	ask("lease-busy", domain.Active, func(f *askFixFixture) { f.rec.writeErr = instance.ErrLeaseBusy }, protoRef(12))
	ask("busy", domain.Active, func(f *askFixFixture) {
		f.svc.guardedSteer.ready = func(*instance.Instance) notReadyReason { return notReadyBusy }
	}, protoRef(12))
	ask("no-source", domain.Active, func(f *askFixFixture) {
		f.svc.guardedSteer.ready = func(*instance.Instance) notReadyReason { return notReadyNoStatusSource }
	}, protoRef(12))

	// Up Next's nudge through the real guard shares the duplicate window with ask-to-fix.
	f := newAskFixFixture(t, domain.Active)
	n := newNudgeFixture(t)
	n.svc.SetPRNudger(f.svc)
	key, err := github.NewPRKey("", "acme", "app", 12)
	require.NoError(t, err)
	n.prs.prs = []github.UserPR{linkedPR("github.com", "acme", "app", 12, "alice", github.LinkedSession{SessionID: "ask-fix"})}
	n.prs.tokens = map[string][2]string{key.String(): {"tok", "alice"}}
	n.fetcher.detail = f.fetcher.detail
	first, err := f.ask(protoRef(12))
	o.record("shared/ask", first, err)
	resp, err := n.svc.NudgeSessionForPR(context.Background(), connect.NewRequest(&sessionv1.NudgeSessionForPRRequest{
		Pr: &sessionv1.PRKey{Owner: "acme", Repo: "app", Number: 12}, SessionId: "ask-fix"}))
	o.record("shared/nudge", msgOf(resp), err)
	o.line("  writes=%d", f.rec.writes)

	const path = "testdata/github_hub.golden"
	if *updateGitHubHubGolden {
		require.NoError(t, os.WriteFile(path, []byte(o.b.String()), 0o600))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), o.b.String())
}
