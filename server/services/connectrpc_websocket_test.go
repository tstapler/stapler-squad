package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/server/protocol"

	"github.com/gorilla/websocket"
)

// createTestWebSocketPair sets up a test WebSocket server and returns the
// server-side connectWebSocketStream and the client-side connection.
func createTestWebSocketPair(t *testing.T) (*connectWebSocketStream, *websocket.Conn, func()) {
	t.Helper()

	streamChan := make(chan *connectWebSocketStream, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("server: failed to upgrade WebSocket: %v", err)
			return
		}
		streamChan <- &connectWebSocketStream{conn: conn}
	}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("failed to connect test client: %v", err)
	}

	serverStream := <-streamChan

	cleanup := func() {
		clientConn.Close()
		serverStream.conn.Close()
		srv.Close()
	}

	return serverStream, clientConn, cleanup
}

// readEnvelopeFromClient reads one binary WebSocket message and parses its envelope.
func readEnvelopeFromClient(t *testing.T, conn *websocket.Conn) *protocol.Envelope {
	t.Helper()
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read message from server: %v", err)
	}
	env, _, err := protocol.ParseEnvelope(msg)
	if err != nil {
		t.Fatalf("failed to parse envelope: %v", err)
	}
	return env
}

// TestSendEndStreamSuccess verifies that sendEndStreamSuccess writes a message
// with the EndStream flag set (regression: streamViaControlMode was missing this call).
func TestSendEndStreamSuccess(t *testing.T) {
	serverStream, clientConn, cleanup := createTestWebSocketPair(t)
	defer cleanup()

	sendEndStreamSuccess(serverStream)

	env := readEnvelopeFromClient(t, clientConn)
	if !env.IsEndStream() {
		t.Errorf("sendEndStreamSuccess: expected EndStream flag (0x%02x), got flags=0x%02x", protocol.EndStreamFlag, env.Flags)
	}
}

// TestSendEndStreamError verifies that sendEndStreamError writes a message
// with the EndStream flag set and an encoded error.
func TestSendEndStreamError(t *testing.T) {
	serverStream, clientConn, cleanup := createTestWebSocketPair(t)
	defer cleanup()

	testErr := fmt.Errorf("something went wrong")
	sendEndStreamError(serverStream, testErr)

	env := readEnvelopeFromClient(t, clientConn)
	if !env.IsEndStream() {
		t.Errorf("sendEndStreamError: expected EndStream flag (0x%02x), got flags=0x%02x", protocol.EndStreamFlag, env.Flags)
	}

	// The payload should be a ConnectRPC JSON error envelope:
	// {"error":{"code":"internal","message":"..."}}
	var errEnvelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(env.Data, &errEnvelope); err != nil {
		t.Fatalf("sendEndStreamError: failed to unmarshal JSON payload: %v", err)
	}
	if errEnvelope.Error.Code != "internal" {
		t.Errorf("sendEndStreamError: expected error code %q, got %q", "internal", errEnvelope.Error.Code)
	}
	if !strings.Contains(errEnvelope.Error.Message, testErr.Error()) {
		t.Errorf("sendEndStreamError: error message %q does not contain %q", errEnvelope.Error.Message, testErr.Error())
	}
}

// TestSendEndStreamSuccessIsIdempotentFormat verifies the envelope structure
// matches what the ConnectRPC client expects (EndStreamFlag = 0x02).
func TestSendEndStreamSuccessEnvelopeFormat(t *testing.T) {
	serverStream, clientConn, cleanup := createTestWebSocketPair(t)
	defer cleanup()

	sendEndStreamSuccess(serverStream)

	_, raw, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read message: %v", err)
	}

	// First byte of envelope is the flags field
	if len(raw) < 5 {
		t.Fatalf("envelope too short: %d bytes", len(raw))
	}
	flags := raw[0]
	if flags&protocol.EndStreamFlag == 0 {
		t.Errorf("EndStream flag (0x%02x) not set in first byte; got 0x%02x", protocol.EndStreamFlag, flags)
	}
}
