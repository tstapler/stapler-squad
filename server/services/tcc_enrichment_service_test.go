package services

import (
	"context"
	"testing"

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

func TestTCCEnrichmentRPC_Disabled(t *testing.T) {
	t.Parallel()
	svc := &SessionService{}
	status, err := svc.GetTCCEnrichmentStatus(context.Background(), connect.NewRequest(&sessionv1.GetTCCEnrichmentStatusRequest{}))
	require.NoError(t, err)
	require.False(t, status.Msg.Enabled)
	require.Empty(t, status.Msg.Roots)
}
