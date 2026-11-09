package tymux

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
	"github.com/tstapler/tymux/clients/go/selecting"

	"github.com/tstapler/stapler-squad/internal/qos"
)

// Real SetPaneActivity sender. It stays inert unless the transport (a selecting client) carries
// the RPC; the daemon capability is checked separately in activityHintAvailable.

// paneActivityTransport is the optional method rpcTransport gains with a newer client.
type paneActivityTransport interface {
	SetPaneActivity(context.Context, *connect.Request[v1.SetPaneActivityRequest]) (*connect.Response[v1.SetPaneActivityResponse], error)
}

// The production transport must carry the RPC, else the hint is silently never sent.
var _ paneActivityTransport = (*selecting.Client)(nil)

type connectActivitySender struct{ t paneActivityTransport }

func (c connectActivitySender) sendPaneActivity(ctx context.Context, paneID string, a qos.Activity) error {
	_, err := c.t.SetPaneActivity(ctx, connect.NewRequest(&v1.SetPaneActivityRequest{PaneId: paneID, Activity: toWireActivity(a)}))
	return err
}

func toWireActivity(a qos.Activity) v1.PaneActivity {
	switch a {
	case qos.ActivityInteractive:
		return v1.PaneActivity_PANE_ACTIVITY_INTERACTIVE
	case qos.ActivityBackground:
		return v1.PaneActivity_PANE_ACTIVITY_BACKGROUND
	default:
		return v1.PaneActivity_PANE_ACTIVITY_UNSPECIFIED
	}
}

func init() {
	newPaneActivitySender = func(t rpcTransport) paneActivitySender {
		if pt, ok := t.(paneActivityTransport); ok {
			return connectActivitySender{pt}
		}
		return nil
	}
}
