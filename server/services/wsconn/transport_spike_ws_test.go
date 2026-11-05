package wsconn

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestTransportSpike_WebSocket_ShouldReportPermessageDeflateNegotiation(t *testing.T) {
	negotiate := func(t *testing.T, up websocket.Upgrader) string {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := up.Upgrade(w, r, nil)
			if err == nil {
				c.Close()
			}
		}))
		defer srv.Close()
		d := websocket.Dialer{EnableCompression: true}
		c, resp, err := d.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
		require.NoError(t, err)
		defer c.Close()
		return resp.Header.Get("Sec-Websocket-Extensions")
	}
	prod := negotiate(t, wsUpgrader)
	enabled := negotiate(t, websocket.Upgrader{EnableCompression: true})
	t.Logf("production wsUpgrader: Sec-WebSocket-Extensions=%q; with EnableCompression: %q", prod, enabled)
	require.Empty(t, prod, "production upgrader does not enable permessage-deflate")
	require.Contains(t, enabled, "permessage-deflate")
}
