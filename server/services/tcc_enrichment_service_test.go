package services

import (
	"context"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tcc"
)

func TestTCCEnrichmentRPC_ConsentAndUnknownRoot(t *testing.T) {
	t.Parallel()
	gate := tcc.NewConsentGate(func(string) tcc.Result { return tcc.Result{Protected: true, Root: "/protected"} }, nil, nil)
	e := session.NewWorktreeEnricher(session.WorktreeEnricherConfig{Ready: gate.Ready, Probe: func(string) (*session.WorktreeInfo, error) { return &session.WorktreeInfo{}, nil }})
	defer e.Stop()
	svc := &SessionService{}
	svc.SetTCCEnrichment(gate, e, true)
	require.False(t, gate.Ready("/protected/repo"))
	status, err := svc.GetTCCEnrichmentStatus(context.Background(), connect.NewRequest(&sessionv1.GetTCCEnrichmentStatusRequest{}))
	require.NoError(t, err)
	require.True(t, status.Msg.Enabled)
	require.Len(t, status.Msg.Roots, 1)
	require.False(t, status.Msg.Roots[0].AppConsented)
	cfg := verdictServiceConfig("localhost:8543", nil)
	ctx := stampedContext(t, &cfg, ListenerLocal, false, "localhost:8543", nil)
	_, err = svc.ContinueTCCEnrichment(ctx, connect.NewRequest(&sessionv1.ContinueTCCEnrichmentRequest{RootId: "/arbitrary"}))
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = svc.ContinueTCCEnrichment(context.Background(), connect.NewRequest(&sessionv1.ContinueTCCEnrichmentRequest{RootId: status.Msg.Roots[0].Id}))
	require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	_, err = svc.ContinueTCCEnrichment(ctx, connect.NewRequest(&sessionv1.ContinueTCCEnrichmentRequest{RootId: status.Msg.Roots[0].Id}))
	require.NoError(t, err)
	require.True(t, gate.Ready("/protected/repo"))
}

func TestTCCEnrichmentRPC_UnparksOnlyConsentedRoots(t *testing.T) {
	t.Parallel()
	var checks atomic.Int32
	gate := tcc.NewConsentGate(func(path string) tcc.Result {
		checks.Add(1)
		return tcc.Result{Protected: true, Root: filepath.Dir(path)}
	}, nil, nil)
	probes := make(chan string, 3)
	e := session.NewWorktreeEnricher(session.WorktreeEnricherConfig{Workers: 1, Ready: gate.Ready, Probe: func(path string) (*session.WorktreeInfo, error) { probes <- path; return &session.WorktreeInfo{}, nil }})
	ctx, cancel := context.WithCancel(context.Background())
	e.Start(ctx)
	t.Cleanup(func() { cancel(); e.Stop() })
	for _, path := range []string{"/first/a", "/first/b", "/other/c"} {
		e.Request(path)
	}
	for checks.Load() != 3 || len(gate.Roots()) != 2 {
		runtime.Gosched()
	}
	require.Empty(t, probes)
	svc := &SessionService{}
	svc.SetTCCEnrichment(gate, e, true)
	roots := gate.Roots()
	var id string
	for _, root := range roots {
		if root.Root == "/first" {
			id = root.ID
		}
	}
	require.NotEmpty(t, id)
	cfg := verdictServiceConfig("localhost:8543", nil)
	writeCtx := stampedContext(t, &cfg, ListenerLocal, false, "localhost:8543", nil)
	_, err := svc.ContinueTCCEnrichment(writeCtx, connect.NewRequest(&sessionv1.ContinueTCCEnrichmentRequest{RootId: id}))
	require.NoError(t, err)
	waitProbe := func() string {
		t.Helper()
		select {
		case path := <-probes:
			return path
		case <-time.After(time.Second):
			t.Fatal("consented path was not unparked")
			return ""
		}
	}
	got := []string{waitProbe(), waitProbe()}
	require.ElementsMatch(t, []string{"/first/a", "/first/b"}, got)
	require.False(t, gate.Ready("/other/c"))
	require.Empty(t, probes)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, err := svc.ContinueTCCEnrichment(writeCtx, connect.NewRequest(&sessionv1.ContinueTCCEnrichmentRequest{RootId: id}))
			require.NoError(t, err)
		})
	}
	wg.Wait()
	require.Empty(t, probes)
}

func TestTCCEnrichmentRPC_Disabled(t *testing.T) {
	t.Parallel()
	svc := &SessionService{}
	status, err := svc.GetTCCEnrichmentStatus(context.Background(), connect.NewRequest(&sessionv1.GetTCCEnrichmentStatusRequest{}))
	require.NoError(t, err)
	require.False(t, status.Msg.Enabled)
	require.Empty(t, status.Msg.Roots)
}
