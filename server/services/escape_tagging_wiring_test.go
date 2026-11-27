package services

import (
	"context"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tagpoller"
)

// The escape RPCs must see the client installed after construction.
func TestSessionService_should_ServeEscapeRPCsFromTheInstalledClient_When_SetAfterConstruction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewSessionService(svctest.CreateTestStorage(t), events.NewEventBus(100))
	t.Cleanup(func() { svc.Shutdown() })
	query := connect.NewRequest(&sessionv1.QueryEscapeAnalyticsRequest{SessionId: "s1"})
	summary := connect.NewRequest(&sessionv1.GetEscapeAnalyticsSummaryRequest{SessionId: "s1"})
	global := connect.NewRequest(&sessionv1.GetEscapeAnalyticsGlobalSummaryRequest{})

	_, err := svc.QueryEscapeAnalytics(ctx, query)
	assertConnectCode(t, err, connect.CodeUnavailable)
	_, err = svc.GetEscapeAnalyticsSummary(ctx, summary)
	assertConnectCode(t, err, connect.CodeUnavailable)
	_, err = svc.GetEscapeAnalyticsGlobalSummary(ctx, global)
	assertConnectCode(t, err, connect.CodeUnavailable)

	client := session.NewTestEntRepository(t).GetEntClient()
	client.EscapeEvent.Create().SetID("e1").SetSessionID("s1").SetStage("pty").SetSequenceType("csi").
		SetByteLength(4).SetSessionSeq(1).SaveX(ctx)
	svc.SetEscapeEventsClient(client)

	events, err := svc.QueryEscapeAnalytics(ctx, query)
	require.NoError(t, err)
	assert.Len(t, events.Msg.Events, 1)
	one, err := svc.GetEscapeAnalyticsSummary(ctx, summary)
	require.NoError(t, err)
	assert.EqualValues(t, 1, one.Msg.TotalSequences)
	all, err := svc.GetEscapeAnalyticsGlobalSummary(ctx, global)
	require.NoError(t, err)
	assert.EqualValues(t, 1, all.Msg.TotalSequences)
}

// modelRecordingPool fails every call and records the model it was asked for.
type modelRecordingPool struct {
	mu     sync.Mutex
	models []string
}

func (p *modelRecordingPool) CallBlocking(_ context.Context, _ headless.FeatureKey, _, _ string, opts headless.CallOptions, sink headless.CostSink) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.models = append(p.models, opts.Model)
	sink(0, true)
	return "", assert.AnError
}

// The classifier RPCs must see the poller installed after construction, and report a missing
// one as Unimplemented rather than calling through a typed nil pointer.
func TestSessionService_should_DriveTheInstalledTagPoller_When_SetAfterConstruction(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	ctx := context.Background()
	svc := NewSessionService(svctest.CreateTestStorage(t), events.NewEventBus(16))
	t.Cleanup(func() { svc.Shutdown() })
	reclassify := connect.NewRequest(&sessionv1.ReclassifySessionTagsRequest{SessionId: "tagged"})
	update := connect.NewRequest(&sessionv1.UpdateTaggingClassifierConfigRequest{
		Config: &sessionv1.TaggingClassifierConfigProto{Model: "sonnet"},
	})

	_, err := svc.ReclassifySessionTags(ctx, reclassify)
	assertConnectCode(t, err, connect.CodeUnimplemented)
	_, err = svc.UpdateTaggingClassifierConfig(ctx, update)
	require.NoError(t, err)
	got, err := svc.GetTaggingClassifierConfig(ctx, connect.NewRequest(&sessionv1.GetTaggingClassifierConfigRequest{}))
	require.NoError(t, err)
	assert.Equal(t, "sonnet", got.Msg.GetConfig().GetModel())

	pool := &modelRecordingPool{}
	poller := tagpoller.NewSessionTagClassificationPoller(pool, classifier.NewTaggingEngine())
	poller.AddInstance(&instance.Instance{Title: "tagged"})
	svc.SetSessionTagPoller(poller)
	update.Msg.Config.Model = "opus"
	_, err = svc.UpdateTaggingClassifierConfig(ctx, update)
	require.NoError(t, err)
	_, err = svc.ReclassifySessionTags(ctx, reclassify)
	assertConnectCode(t, err, connect.CodeUnavailable)
	assert.Equal(t, []string{"opus"}, pool.models, "Update must hot-apply the model to the running poller")
}
