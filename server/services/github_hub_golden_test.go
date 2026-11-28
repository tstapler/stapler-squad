package services

// Differential golden for the hub's side of the GitHub nudge cut: ask-to-fix, and
// Up Next's nudge through the real SessionService guard. Recorded before the move.

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/server/services/githubrpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

var updateGitHubHubGolden = flag.Bool("update-github-hub-golden", false, "rewrite testdata/github_hub.golden")

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

func TestGitHubHubGolden(t *testing.T) {
	t.Parallel()
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
	key, err := github.NewPRKey("", "acme", "app", 12)
	require.NoError(t, err)
	userSvc := githubrpc.NewGitHubUserService(nil, nil)
	userSvc.SetPRNudger(f.svc)
	userSvc.SetPRDetailFetcher(&fakeFetcher{detail: f.fetcher.detail})
	userSvc.SetPRTokenResolver(staticPRTokens{key.String(): "tok"})
	userSvc.SetPRSnapshotSource(fixedUserPRs{{Host: "github.com", Owner: "acme", Repo: "app", Number: 12,
		LinkedSessions: []github.LinkedSession{{SessionID: "ask-fix"}}}})
	first, err := f.ask(protoRef(12))
	o.record("shared/ask", first, err)
	resp, err := userSvc.NudgeSessionForPR(context.Background(), connect.NewRequest(&sessionv1.NudgeSessionForPRRequest{
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
