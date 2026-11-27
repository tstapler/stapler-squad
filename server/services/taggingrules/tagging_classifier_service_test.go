package taggingrules

import (
	"context"
	"errors"
	"strings"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tagpoller"
)

// isolatedClassifierService builds a poller-less ClassifierService with config I/O redirected
// to a temp dir, so UpdateTaggingClassifierConfig's SaveConfig never touches the real config.json.
func isolatedClassifierService(t *testing.T) *ClassifierService {
	t.Helper()
	svctest.WithIsolatedStateDir(t)
	return NewClassifierService(nil)
}

func assertConnectCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	var ce *connect.Error
	require.ErrorAs(t, err, &ce, "expected a connect.Error")
	assert.Equal(t, want, ce.Code(), "expected connect code %v, got %v", want, ce.Code())
}

func TestTaggingClassifierService_should_ReturnHaikuDefault_When_Unconfigured(t *testing.T) {
	svc := isolatedClassifierService(t)

	resp, err := svc.GetTaggingClassifierConfig(context.Background(), connect.NewRequest(&sessionv1.GetTaggingClassifierConfigRequest{}))
	require.NoError(t, err)
	assert.Equal(t, "haiku", resp.Msg.GetConfig().GetModel())
	assert.Empty(t, resp.Msg.GetConfig().GetFallbackModels())
	assert.False(t, resp.Msg.GetEnvOverrideActive())
}

func TestTaggingClassifierService_should_PersistAndEchoHierarchy_When_Updated(t *testing.T) {
	svc := isolatedClassifierService(t)

	update, err := svc.UpdateTaggingClassifierConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateTaggingClassifierConfigRequest{
		Config: &sessionv1.TaggingClassifierConfigProto{Model: "sonnet", FallbackModels: []string{"proxy-free", "", "sonnet", "  "}},
	}))
	require.NoError(t, err)
	assert.Equal(t, "sonnet", update.Msg.GetConfig().GetModel())
	// Blanks dropped, duplicate-of-primary dropped.
	assert.Equal(t, []string{"proxy-free"}, update.Msg.GetConfig().GetFallbackModels())

	// A fresh read must observe the persisted hierarchy (proves SaveConfig, not echo).
	got, err := svc.GetTaggingClassifierConfig(context.Background(), connect.NewRequest(&sessionv1.GetTaggingClassifierConfigRequest{}))
	require.NoError(t, err)
	assert.Equal(t, "sonnet", got.Msg.GetConfig().GetModel())
	assert.Equal(t, []string{"proxy-free"}, got.Msg.GetConfig().GetFallbackModels())
}

func TestTaggingClassifierService_should_ResetToDefault_When_ModelBlank(t *testing.T) {
	svc := isolatedClassifierService(t)

	_, err := svc.UpdateTaggingClassifierConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateTaggingClassifierConfigRequest{
		Config: &sessionv1.TaggingClassifierConfigProto{Model: "sonnet"},
	}))
	require.NoError(t, err)

	_, err = svc.UpdateTaggingClassifierConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateTaggingClassifierConfigRequest{
		Config: &sessionv1.TaggingClassifierConfigProto{Model: "   "},
	}))
	require.NoError(t, err)

	got, err := svc.GetTaggingClassifierConfig(context.Background(), connect.NewRequest(&sessionv1.GetTaggingClassifierConfigRequest{}))
	require.NoError(t, err)
	assert.Equal(t, "haiku", got.Msg.GetConfig().GetModel())
}

func TestTaggingClassifierService_should_RejectInvalidConfig_When_Updated(t *testing.T) {
	svc := isolatedClassifierService(t)

	_, err := svc.UpdateTaggingClassifierConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateTaggingClassifierConfigRequest{}))
	assertConnectCode(t, err, connect.CodeInvalidArgument)

	_, err = svc.UpdateTaggingClassifierConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateTaggingClassifierConfigRequest{
		Config: &sessionv1.TaggingClassifierConfigProto{Model: strings.Repeat("x", 129)},
	}))
	assertConnectCode(t, err, connect.CodeInvalidArgument)

	_, err = svc.UpdateTaggingClassifierConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateTaggingClassifierConfigRequest{
		Config: &sessionv1.TaggingClassifierConfigProto{
			Model:          "haiku",
			FallbackModels: []string{"a", "b", "c", "d", "e", "f"},
		},
	}))
	assertConnectCode(t, err, connect.CodeInvalidArgument)
}

func TestTaggingClassifierService_should_PreferEnv_When_EnvOverridesSet(t *testing.T) {
	svc := isolatedClassifierService(t)
	t.Setenv("STAPLER_SQUAD_TAGGING_MODEL", "proxy-free")
	t.Setenv("STAPLER_SQUAD_TAGGING_FALLBACK_MODELS", "haiku, sonnet")

	got, err := svc.GetTaggingClassifierConfig(context.Background(), connect.NewRequest(&sessionv1.GetTaggingClassifierConfigRequest{}))
	require.NoError(t, err)
	assert.Equal(t, "proxy-free", got.Msg.GetConfig().GetModel())
	assert.Equal(t, []string{"haiku", "sonnet"}, got.Msg.GetConfig().GetFallbackModels())
	assert.True(t, got.Msg.GetEnvOverrideActive())
}

func TestTaggingClassifierService_should_LiveUpdatePoller_When_Updated(t *testing.T) {
	svctest.WithIsolatedStateDir(t)

	// Attach a poller with no sessions: Update must still hot-apply without error, and a
	// later Get must report the new hierarchy (persistence + live path together).
	svc := NewClassifierService(tagpoller.NewSessionTagClassificationPoller(nil, nil))
	_, err := svc.UpdateTaggingClassifierConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateTaggingClassifierConfigRequest{
		Config: &sessionv1.TaggingClassifierConfigProto{Model: "sonnet", FallbackModels: []string{"proxy-free"}},
	}))
	require.NoError(t, err)
	got, err := svc.GetTaggingClassifierConfig(context.Background(), connect.NewRequest(&sessionv1.GetTaggingClassifierConfigRequest{}))
	require.NoError(t, err)
	assert.Equal(t, "sonnet", got.Msg.GetConfig().GetModel())
}

func TestTaggingClassifierService_should_RejectReclassify_When_BadRequest(t *testing.T) {
	svc := isolatedClassifierService(t)

	_, err := svc.ReclassifySessionTags(context.Background(), connect.NewRequest(&sessionv1.ReclassifySessionTagsRequest{}))
	assertConnectCode(t, err, connect.CodeInvalidArgument)

	// No poller attached (no headless pool): explicit Unimplemented, not a nil panic.
	_, err = svc.ReclassifySessionTags(context.Background(), connect.NewRequest(&sessionv1.ReclassifySessionTagsRequest{SessionId: "sess-1"}))
	assertConnectCode(t, err, connect.CodeUnimplemented)

	// Poller attached but session unknown: NotFound.
	svc = NewClassifierService(tagpoller.NewSessionTagClassificationPoller(nil, nil))
	_, err = svc.ReclassifySessionTags(context.Background(), connect.NewRequest(&sessionv1.ReclassifySessionTagsRequest{SessionId: "no-such-session"}))
	assertConnectCode(t, err, connect.CodeNotFound)
}

func TestTaggingClassifierService_should_ReportEnvOverride_When_Updated(t *testing.T) {
	svc := isolatedClassifierService(t)
	t.Setenv("STAPLER_SQUAD_TAGGING_MODEL", "env-model")
	t.Setenv("STAPLER_SQUAD_TAGGING_FALLBACK_MODELS", "env-fb")

	update, err := svc.UpdateTaggingClassifierConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateTaggingClassifierConfigRequest{
		Config: &sessionv1.TaggingClassifierConfigProto{Model: "sonnet", FallbackModels: []string{"proxy-free"}},
	}))
	require.NoError(t, err)
	assert.Equal(t, "env-model", update.Msg.GetConfig().GetModel())
	assert.Equal(t, []string{"env-fb"}, update.Msg.GetConfig().GetFallbackModels())
}

// errTagPoolClient is a headless.PoolClient whose calls fail when err is set, else return response.
type errTagPoolClient struct {
	response string
	err      error
}

func (f *errTagPoolClient) CallBlocking(_ context.Context, _ headless.FeatureKey, _, _ string, _ headless.CallOptions, sink headless.CostSink) (string, error) {
	sink(0, true)
	return f.response, f.err
}

// reclassifyFixture registers one unstarted session with a poller backed by client.
func reclassifyFixture(t *testing.T, client headless.PoolClient, title string) *ClassifierService {
	t.Helper()
	svctest.WithIsolatedStateDir(t)
	poller := tagpoller.NewSessionTagClassificationPoller(client, classifier.NewTaggingEngine())
	poller.AddInstance(&instance.Instance{Title: title})
	return NewClassifierService(poller)
}

func TestTaggingClassifierService_should_ReturnTags_When_ReclassifySucceeds(t *testing.T) {
	const title = "reclassify-ok-session"
	svc := reclassifyFixture(t, &errTagPoolClient{response: `{"results":[{"name":"` + title + `","tags":["Unclassified"]}]}`}, title)

	resp, err := svc.ReclassifySessionTags(context.Background(), connect.NewRequest(&sessionv1.ReclassifySessionTagsRequest{SessionId: title}))

	require.NoError(t, err)
	assert.Contains(t, resp.Msg.GetTags(), instance.UnclassifiedTag)
}

func TestTaggingClassifierService_should_ReturnUnavailable_When_ReclassifyDegrades(t *testing.T) {
	const title = "reclassify-degraded-session"
	svc := reclassifyFixture(t, &errTagPoolClient{err: errors.New("pool down")}, title)

	_, err := svc.ReclassifySessionTags(context.Background(), connect.NewRequest(&sessionv1.ReclassifySessionTagsRequest{SessionId: title}))

	assertConnectCode(t, err, connect.CodeUnavailable)
}
