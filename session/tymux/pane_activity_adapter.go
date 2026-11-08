//go:build tymux_pane_activity

package tymux

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/internal/qos"
)

// This file is the real SetPaneActivity sender. It needs a tymux client whose generated code has
// the RPC (tymux#86) AND whose selecting.Transport/Client forwards it; the pinned client has
// neither, so the file is excluded from default builds. When the pin moves, drop the build tag
// (and this paragraph): nothing else changes.

// paneActivityTransport is the optional method rpcTransport gains with a newer client.
type paneActivityTransport interface {
	SetPaneActivity(context.Context, *connect.Request[v1.SetPaneActivityRequest]) (*connect.Response[v1.SetPaneActivityResponse], error)
}

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
		return v1.PaneActivity_PANE_ACTIVITY_NORMAL
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
