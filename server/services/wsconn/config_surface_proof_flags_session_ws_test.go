package wsconn

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/protocol"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
)

func TestConfigSurfaceProof_TerminalResyncFlags_ChangeResyncOptions(t *testing.T) {
	set := bkfFlagWriter(t)
	for name, field := range map[string]func(ResyncOptions) bool{
		services.TerminalResyncSkipStaleDimensionSlowpathFlagName: func(o ResyncOptions) bool { return o.SkipStaleDimensionSlowPath },
		services.TerminalResyncExecGateFastLaneFlagName:           func(o ResyncOptions) bool { return o.UseFastLane },
		services.TerminalResyncCorrelationIDFlagName:              func(o ResyncOptions) bool { return o.EchoResyncID },
	} {
		t.Run(name, func(t *testing.T) {
			svctest.AssertConfigChangesBehavior(t, func(setting any) any {
				set(name, setting.(bool))
				return field(currentResyncOptions())
			}, false, true)
		})
	}
}

func TestConfigSurfaceProof_TerminalResyncCompression_ChangesEnvelopeFlag(t *testing.T) {
	set := bkfFlagWriter(t)
	largeContent := bytes.Repeat([]byte("resync-payload-filler-"), 100)

	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set(services.TerminalResyncCompressionFlagName, setting.(bool))
		stream, clientConn, cleanup := createTestWebSocketPair(t)
		defer cleanup()
		target := &fakePanePTY{captureContent: string(largeContent), cols: 80, rows: 24}
		onCurrentPane := func(ctx context.Context, r *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
			return handleCurrentPaneRequest(ctx, "bkf-session", target, r, currentResyncOptionsFor(testWriter()))
		}
		var settling atomic.Bool
		handleCurrentPaneRequestFrame(stream, "bkf-session", &sessionv1.CurrentPaneRequest{}, onCurrentPane, &settling)
		return readEnvelopeFromClient(t, clientConn).Flags&protocol.CompressedFlag != 0
	}, false, true)
}

func TestConfigSurfaceProof_TerminalAppScrollbackForwardingClaude_ChangesScrollbackPath(t *testing.T) {
	set := bkfFlagWriter(t)
	inst := session.NewStartedInstanceForTest(t, "bkf-scroll", &replyPM{})
	inst.Program = "claude"

	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set(config.FeatureAppScrollForwardingClaude, setting.(bool))
		result, err := scrollbackResultForRequest(scrollbackRequestParams{
			writer:          testWriter(),
			instance:        inst,
			subscriberCount: -1,
			fallback:        func(string, string) (string, error) { return "tmux-native", nil },
		})
		require.NoError(t, err)
		return result.AppScroll != nil
	}, false, true)
}
