package wsconn

import (
	"context"
	"fmt"

	"github.com/gorilla/websocket"
	"github.com/tstapler/stapler-squad/server/protocol"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session/hostclaim"
	"github.com/tstapler/stapler-squad/session/streamhub"
	"google.golang.org/protobuf/proto"
)

// streamFederatedTerminal shows a peer's session in the browser's terminal,
// read-only: browser input and resize frames are discarded, never forwarded.
func (h *ConnectRPCWebSocketHandler) streamFederatedTerminal(stream *connectWebSocketStream, id hostclaim.FederatedSessionID, wireID string) error {
	fed := h.sessionService.Federation()
	if fed == nil {
		return fmt.Errorf("%w: federation is not enabled", services.ErrFederatedHostUnavailable)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Drain the browser so a disconnect is noticed; its frames are read-only-dropped.
	go func() {
		defer cancel()
		for {
			if _, _, err := stream.conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	return fed.StreamTerminal(ctx, id, func(content string) error {
		frame := newTerminalOutputData(wireID, ansiSnapshotPrefix+prepareSnapshotContent(streamhub.RawPaneContent(content)), false)
		b, err := proto.Marshal(frame)
		if err != nil {
			return err
		}
		return stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, b))
	})
}
