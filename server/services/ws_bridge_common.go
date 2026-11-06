package services

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// wsCloseWriteWait bounds a close-frame write.
const wsCloseWriteWait = 2 * time.Second

// upgradeWebSocket upgrades r with the caller's origin rule. The stream bridges
// pass isAllowedOrigin and readLimit 0 (gorilla default, no limit); the unary
// bridge passes its per-listener policy and a hard read backstop.
func upgradeWebSocket(w http.ResponseWriter, r *http.Request, checkOrigin func(*http.Request) bool, readLimit int64) (*websocket.Conn, error) {
	up := websocket.Upgrader{
		ReadBufferSize:  wsUpgrader.ReadBufferSize,
		WriteBufferSize: wsUpgrader.WriteBufferSize,
		CheckOrigin:     checkOrigin,
	}
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	if readLimit > 0 {
		conn.SetReadLimit(readLimit)
	}
	return conn, nil
}

// closeWebSocket sends a close frame with code and reason; safe to call
// concurrently with the connection's writer (gorilla WriteControl).
func closeWebSocket(conn *websocket.Conn, code int, reason string) {
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(wsCloseWriteWait))
}
