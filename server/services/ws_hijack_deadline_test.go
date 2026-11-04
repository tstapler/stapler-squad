package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// http.Server.ReadTimeout sets a read deadline on the connection before the handler
// runs, and Hijack hands that deadline to the caller. This pins that gorilla's Upgrade
// clears it, so a terminal socket is not cut at the server's ReadTimeout (15s in
// production, server.go httpServer). The client stays silent for longer than
// ReadTimeout before sending its first frame.
func TestWSUpgrade_should_ClearServerReadTimeout_When_ConnectionIsHijacked(t *testing.T) {
	const readTimeout = 200 * time.Millisecond

	received := make(chan string, 1)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, msg, err := conn.ReadMessage()
		if err != nil {
			received <- "error: " + err.Error()
			return
		}
		received <- string(msg)
	}))
	srv.Config.ReadTimeout = readTimeout
	srv.Start()
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	defer client.Close()

	<-time.After(3 * readTimeout) // idle well past ReadTimeout
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte("hello")))

	select {
	case got := <-received:
		require.Equal(t, "hello", got, "the hijacked connection must survive the server's ReadTimeout")
	case <-time.After(5 * time.Second):
		t.Fatal("handler never received the frame")
	}
}
