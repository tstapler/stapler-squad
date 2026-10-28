package services

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// Run with: go test ./server/services -run WsUnary -race
func TestWsUnaryIntegration_ShouldPass_WhenRunWithRace(t *testing.T) {
	for _, tc := range []struct {
		name   string
		useTLS bool
	}{{"ws", false}, {"wss", true}} {
		t.Run(tc.name, func(t *testing.T) {
			h := newBridgeHarnessTLS(t, ListenerLocal, tc.useTLS)
			scheme := "http://"
			if tc.useTLS {
				scheme = "https://"
			}
			c := h.connect(http.Header{"Origin": {scheme + "localhost"}})

			// Multiplex: the fast call answers before the slow one.
			sendListFiles(t, c, 1, "gate:slow")
			<-h.stub.started
			sendListFiles(t, c, 3, "fast")
			f := readFrame(t, c)
			assert.EqualValues(t, 3, f.ID)
			assert.Equal(t, "ok:fast", decodeListFiles(t, f).BasePath)
			h.stub.release("gate:slow")
			assert.EqualValues(t, 1, readFrame(t, c).ID)

			// Cancel: the handler sees cancellation and no frame is written for the id.
			sendListFiles(t, c, 5, "gate:c")
			<-h.stub.started
			require.NoError(t, c.WriteMessage(websocket.BinaryMessage, encodeUnaryControl(unaryFrameCancel, 5)))
			assert.Equal(t, "gate:c", <-h.stub.canceled)

			// Deadline.
			sendRequest(t, c, 7, listFilesMethod, unaryRequestMeta{TimeoutMs: 50},
				mustProto(t, &sessionv1.ListFilesRequest{SessionId: "gate:d"}))
			assert.Equal(t, "deadline_exceeded", readFrame(t, c).Error.Code)

			// Deny-by-default code.
			sendRequest(t, c, 9, writeFileMethod, unaryRequestMeta{}, nil)
			assert.Equal(t, codePermissionDenied, readFrame(t, c).Error.Code)

			// In-flight cap: the call beyond the cap is refused while the socket stays open.
			for i := 1; i <= wsUnaryMaxInFlight; i++ {
				sendListFiles(t, c, uint32(100+2*i), "gate:cap"+strconv.Itoa(i))
				<-h.stub.started
			}
			sendListFiles(t, c, 999, "gate:over")
			f = readFrame(t, c)
			assert.EqualValues(t, 999, f.ID)
			assert.Equal(t, codeResourceExhausted, f.Error.Code)

			// Malformed frame closes with 1003.
			require.NoError(t, c.WriteMessage(websocket.BinaryMessage, []byte{1, 0}))
			require.NoError(t, c.SetReadDeadline(time.Now().Add(readWait)))
			for {
				_, _, err := c.ReadMessage()
				if err != nil {
					assert.True(t, websocket.IsCloseError(err, websocket.CloseUnsupportedData), "got %v", err)
					break
				}
			}
		})
	}
}
