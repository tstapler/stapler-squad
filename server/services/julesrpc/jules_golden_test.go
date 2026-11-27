package julesrpc

// Differential golden for the Jules config RPCs, dispatch guards and usage
// counter, recorded from origin/main before the move.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/jules"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

var updateJulesGolden = flag.Bool("update-jules-golden", false, "rewrite testdata/jules.golden")

type goldenSources struct{}

func (goldenSources) Resolve(_ context.Context, owner, repo string) (jules.JulesSourceName, error) {
	if repo == "unknown" {
		return "", fmt.Errorf("%w: %s/%s", jules.ErrJulesSourceNotRegistered, owner, repo)
	}
	return "sources/x", nil
}

type goldenReconnect bool

func (g goldenReconnect) AuthReconnectRequired() bool { return bool(g) }

func TestJulesGolden(t *testing.T) {
	// The config RPCs read and write config.json, so this test owns HOME and the state dir.
	svctest.WithFakeHome(t)
	svctest.WithIsolatedStateDir(t)
	ctx := context.Background()
	var out strings.Builder
	record := func(step string, msg proto.Message, err error) {
		if err != nil {
			fmt.Fprintf(&out, "%s err code=%v %v\n", step, connect.CodeOf(err), err)
			return
		}
		b, merr := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(msg)
		require.NoError(t, merr)
		fmt.Fprintf(&out, "%s %s\n", step, strings.ReplaceAll(string(b), " ", ""))
	}
	usage := NewJulesUsageCounter()
	usage.IncSessionDispatched()
	usage.IncSessionCompleted()
	usage.IncSessionCompleted()
	usage.IncSessionFailed()
	usage.IncAPIRateLimited()
	usage.IncAPIError()
	fmt.Fprintf(&out, "usage %+v\n", usage.Snapshot())

	bare := NewJulesConfigService(nil, nil, nil)
	wired := NewJulesConfigService(&fakeJulesKeyManager{}, goldenSources{}, goldenReconnect(true))
	wired.SetUsageCounter(usage)
	for _, tc := range []struct {
		name string
		svc  *JulesConfigService
	}{{"bare", bare}, {"wired", wired}} {
		fmt.Fprintf(&out, "=== %s\n", tc.name)
		get := func(step string) {
			resp, err := tc.svc.GetJulesConfig(ctx, connect.NewRequest(&sessionv1.GetJulesConfigRequest{}))
			require.NoError(t, err)
			record(step, resp.Msg, nil)
		}
		get("get")
		for _, key := range []string{"", "__clear__", "bad key", "AIzaSyD-EXAMPLE"} {
			resp, err := tc.svc.UpdateJulesConfig(ctx, connect.NewRequest(&sessionv1.UpdateJulesConfigRequest{
				ApiKey: key, Enabled: true, MaxConcurrentJulesSessions: 3, MaxJulesSessionsPerDay: 9,
			}))
			if err != nil {
				record("update "+key, nil, err)
				continue
			}
			record("update "+key, resp.Msg, nil)
		}
		for _, repo := range []string{"", "https://github.com/o/r", "https://github.com/o/unknown"} {
			resp, err := tc.svc.TestJulesConnection(ctx, connect.NewRequest(&sessionv1.TestJulesConnectionRequest{RepoPath: repo}))
			if err != nil {
				record("test "+repo, nil, err)
				continue
			}
			record("test "+repo, resp.Msg, nil)
		}
		for _, repo := range []string{"", "/r/a", "/r/a", "/r/b"} {
			resp, err := tc.svc.ConfirmEgressConsent(ctx, connect.NewRequest(&sessionv1.ConfirmEgressConsentRequest{RepoPath: repo}))
			if err != nil {
				record("confirm "+repo, nil, err)
				continue
			}
			record("confirm "+repo, resp.Msg, nil)
		}
		for _, repo := range []string{"", "/r/a", "/r/a", "/r/b"} {
			resp, err := tc.svc.RevokeEgressConsent(ctx, connect.NewRequest(&sessionv1.RevokeEgressConsentRequest{RepoPath: repo}))
			if err != nil {
				record("revoke "+repo, nil, err)
				continue
			}
			record("revoke "+repo, resp.Msg, nil)
		}
		get("get-after")
	}

	out.WriteString("=== dispatch\n")
	for _, r := range [][3]string{{"", "main", "p"}, {"id", "", "p"}, {"id", "bad branch", "p"}, {"id", "main", ""}, {"id", "main", "p"}} {
		req, err := NewJulesDispatchRequest(r[0], r[1], r[2])
		fmt.Fprintf(&out, "request %q -> %+v err=%v\n", r, req, err)
	}
	storage := svctest.CreateTestStorage(t)
	item := svctest.CreateTestJulesReadyItem(t, ctx, storage, "golden item")
	req, err := NewJulesDispatchRequest(item.ID, "main", "do it")
	require.NoError(t, err)
	clean := func(s string) string {
		s = strings.ReplaceAll(s, item.ID, "<item>")
		return strings.ReplaceAll(s, item.RepoPath, "<repo>")
	}
	guard := &fakeJulesTransitionGuard{}
	dispatch := func(step string, cfg *config.Config, client *svctest.FakeJulesSessionCreator) {
		svc := NewJulesDispatchService(storage, guard, client, svctest.NewTestJulesSourceRegistry(), func() *config.Config { return cfg })
		counter := NewJulesUsageCounter()
		svc.SetUsageCounter(counter)
		name, err := svc.DispatchToJules(ctx, item, req)
		open, lerr := storage.ListOpenJulesItemSessions(ctx)
		require.NoError(t, lerr)
		fmt.Fprintf(&out, "%s name=%q err=%s inFlight=%t egress=%t calls=%d open=%d usage=%+v\n", step, name,
			clean(fmt.Sprint(err)), errors.Is(err, ErrJulesDispatchInFlight), errors.Is(err, ErrJulesEgressNotAcknowledged),
			client.CallCount(), len(open), counter.Snapshot())
	}
	dispatch("nil-config", nil, &svctest.FakeJulesSessionCreator{})
	dispatch("disabled", &config.Config{}, &svctest.FakeJulesSessionCreator{})
	dispatch("no-consent", svctest.NewTestJulesConfig("/elsewhere"), &svctest.FakeJulesSessionCreator{})
	dispatch("rate-limited", svctest.NewTestJulesConfig(item.RepoPath), &svctest.FakeJulesSessionCreator{Err: jules.ErrJulesRateLimited})
	dispatch("transient", svctest.NewTestJulesConfig(item.RepoPath), &svctest.FakeJulesSessionCreator{Err: jules.ErrJulesTransient})
	dispatch("success", svctest.NewTestJulesConfig(item.RepoPath), &svctest.FakeJulesSessionCreator{ResultName: "sessions/golden"})
	dispatch("again", svctest.NewTestJulesConfig(item.RepoPath), &svctest.FakeJulesSessionCreator{})
	fmt.Fprintf(&out, "transitions=%d\n", guard.callCount())

	const path = "testdata/jules.golden"
	if *updateJulesGolden {
		require.NoError(t, os.WriteFile(path, []byte(out.String()), 0o600))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), out.String())
}
