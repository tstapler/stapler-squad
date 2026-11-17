package services

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tcc"
)

func TestTCCEnrichmentRPC_ConsentAndUnknownRoot(t *testing.T) {
	t.Parallel()
	gate := tcc.NewConsentGate(func(string) tcc.Result { return tcc.Result{Protected: true, Root: "/protected"} }, nil, nil)
	e := instance.NewWorktreeEnricher(instance.WorktreeEnricherConfig{Ready: gate.Ready, Probe: func(string) (*instance.WorktreeInfo, error) { return &instance.WorktreeInfo{}, nil }})
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
	e := instance.NewWorktreeEnricher(instance.WorktreeEnricherConfig{Workers: 1, Ready: gate.Ready, Probe: func(path string) (*instance.WorktreeInfo, error) {
		probes <- path
		return &instance.WorktreeInfo{}, nil
	}})
	ctx, cancel := context.WithCancel(context.Background())
	e.Start(ctx)
	t.Cleanup(func() { cancel(); e.Stop() })
	for _, path := range []string{"/first/a", "/first/b", "/other/c"} {
		e.Request(path)
	}
	require.Eventually(t, func() bool {
		return checks.Load() == 3 && len(gate.Roots()) == 2
	}, time.Second, time.Millisecond, "enrichment paths were not parked")
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

type tccDenialFixture struct {
	gate  *tcc.ConsentGate
	e     *instance.WorktreeEnricher
	svc   *SessionService
	clock *atomic.Int64
	err   atomic.Value // holds a boxed error returned by the probe
	probe chan struct{}
}

type boxedErr struct{ err error }

func newTCCDenialFixture(t *testing.T, probeErr error) *tccDenialFixture {
	t.Helper()
	f := &tccDenialFixture{clock: &atomic.Int64{}, probe: make(chan struct{}, 8)}
	f.err.Store(boxedErr{probeErr})
	base := time.Unix(1_700_000_000, 0)
	f.gate = tcc.NewConsentGate(func(string) tcc.Result {
		return tcc.Result{Protected: true, Root: "/protected", Service: tcc.FilesAndFolders}
	}, nil, nil)
	f.e = instance.NewWorktreeEnricher(instance.WorktreeEnricherConfig{
		Ready:     f.gate.Ready,
		ResultTTL: time.Minute,
		Now:       func() time.Time { return base.Add(time.Duration(f.clock.Load())) },
		OnResult:  f.gate.ObserveResult,
		Probe: func(string) (*instance.WorktreeInfo, error) {
			defer func() { f.probe <- struct{}{} }()
			if err := f.err.Load().(boxedErr).err; err != nil {
				return nil, err
			}
			return &instance.WorktreeInfo{}, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	f.e.Start(ctx)
	t.Cleanup(func() { cancel(); f.e.Stop() })
	f.svc = &SessionService{}
	f.svc.SetTCCEnrichment(f.gate, f.e, true)
	f.e.Request("/protected/repo")
	require.Eventually(t, func() bool { return len(f.gate.Roots()) == 1 }, time.Second, time.Millisecond)
	_, err := f.gate.Continue(f.gate.Roots()[0].ID)
	require.NoError(t, err)
	f.e.Unpark("/protected/repo")
	f.waitProbe(t)
	return f
}

func (f *tccDenialFixture) waitProbe(t *testing.T) {
	t.Helper()
	select {
	case <-f.probe:
	case <-time.After(time.Second):
		t.Fatal("probe did not run")
	}
}

func (f *tccDenialFixture) root(t *testing.T) *sessionv1.TCCEnrichmentRoot {
	t.Helper()
	status, err := f.svc.GetTCCEnrichmentStatus(context.Background(), connect.NewRequest(&sessionv1.GetTCCEnrichmentStatusRequest{}))
	require.NoError(t, err)
	require.Len(t, status.Msg.Roots, 1)
	return status.Msg.Roots[0]
}

func TestTCCEnrichmentRPC_EPERMIsStickyDeniedAndRecheckClearsIt(t *testing.T) {
	t.Parallel()
	f := newTCCDenialFixture(t, fmt.Errorf("open: %w", syscall.EPERM))
	require.Eventually(t, func() bool { return f.root(t).Outcome == "denied" }, time.Second, time.Millisecond)
	require.Equal(t, tcc.SettingsURLFilesAndFolders, f.root(t).SettingsUrl)

	f.clock.Add(int64(10 * time.Minute)) // result TTL (1m) expires; denial must not flap
	require.Equal(t, "denied", f.root(t).Outcome)

	f.err.Store(boxedErr{nil}) // user granted access
	cfg := verdictServiceConfig("localhost:8543", nil)
	ctx := stampedContext(t, &cfg, ListenerLocal, false, "localhost:8543", nil)
	_, err := f.svc.ContinueTCCEnrichment(ctx, connect.NewRequest(&sessionv1.ContinueTCCEnrichmentRequest{RootId: f.root(t).Id}))
	require.NoError(t, err)
	f.waitProbe(t)
	require.Eventually(t, func() bool {
		r := f.root(t)
		return r.Outcome == "completed" && r.SettingsUrl == ""
	}, time.Second, time.Millisecond)
}

func TestTCCEnrichmentRPC_EACCESIsOrdinaryErrorNotDenied(t *testing.T) {
	t.Parallel()
	f := newTCCDenialFixture(t, fmt.Errorf("open: %w", syscall.EACCES))
	require.Eventually(t, func() bool { return f.root(t).Outcome == "error" }, time.Second, time.Millisecond)
	require.Empty(t, f.root(t).SettingsUrl)
}
