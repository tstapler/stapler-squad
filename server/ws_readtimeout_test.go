package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// TestWebSocketSurvivesServerReadTimeout pins that the http.Server ReadTimeout
// (newServerBase sets 15s) does not close an upgraded WebSocket. Both net/http's
// Hijack and gorilla's Upgrade clear the connection deadlines, so this test cannot
// say which one does; it uses a plain upgrader, not the app's real handler chain. Uses a short timeout so it needs no 15s wait.
func TestWebSocketSurvivesServerReadTimeout(t *testing.T) {
	const readTimeout = 150 * time.Millisecond
	up := websocket.Upgrader{}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			mt, msg, err := c.ReadMessage()
			if err != nil {
				return
			}
			if c.WriteMessage(mt, msg) != nil {
				return
			}
		}
	}))
	srv.Config.ReadTimeout = readTimeout
	srv.Start()
	defer srv.Close()

	c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	defer c.Close()

	<-time.After(3 * readTimeout) // real elapsed time past ReadTimeout is the thing under test
	require.NoError(t, c.WriteMessage(websocket.TextMessage, []byte("ping")))
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := c.ReadMessage()
	require.NoError(t, err, "socket must outlive http.Server.ReadTimeout")
	require.Equal(t, "ping", string(msg))
}
