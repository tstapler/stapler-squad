package cdp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// fakeScreencastChrome mimics Chrome's split endpoints: the browser-level socket rejects
// Page.* (as real Chrome does) and only the page socket serves the screencast.
func fakeScreencastChrome(t *testing.T, frame []byte) (port int, pageCommands <-chan string) {
	t.Helper()
	cmds := make(chan string, 16)
	up := websocket.Upgrader{}
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"webSocketDebuggerUrl":"ws://%s/devtools/browser/b1"}`, base)
	})
	mux.HandleFunc("/json/list", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `[{"type":"service_worker","webSocketDebuggerUrl":"ws://%[1]s/devtools/page/sw"},{"type":"page","webSocketDebuggerUrl":"ws://%[1]s/devtools/page/p1"}]`, base)
	})
	mux.HandleFunc("/devtools/browser/b1", func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			var m cdpMessage
			if c.ReadJSON(&m) != nil {
				return
			}
			_ = c.WriteJSON(map[string]any{"id": m.ID, "error": map[string]any{"code": -32601, "message": "'" + m.Method + "' wasn't found"}})
		}
	})
	mux.HandleFunc("/devtools/page/p1", func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			var m cdpMessage
			if c.ReadJSON(&m) != nil {
				return
			}
			cmds <- m.Method
			_ = c.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
			if m.Method == "Page.startScreencast" {
				_ = c.WriteJSON(map[string]any{"method": "Page.screencastFrame", "params": map[string]any{"data": base64.StdEncoding.EncodeToString(frame), "sessionId": 7}})
			}
		}
	})
	srv := httptest.NewUnstartedServer(mux)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv.Listener = l
	srv.Start()
	t.Cleanup(srv.Close)
	base = l.Addr().String()
	return l.Addr().(*net.TCPAddr).Port, cmds
}

func TestScreencast_UsesPageTargetAndStreamsOnFirstFrame(t *testing.T) {
	frame := []byte("\xff\xd8fake-jpeg")
	port, cmds := fakeScreencastChrome(t, frame)

	m := &cdpStreamManager{cfg: CDPConfig{SessionID: "t"}, state: CDPState{Status: CDPStatusNoBrowser, Port: port}}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go m.runLoop(ctx, port)

	deadline := time.After(5 * time.Second)
	for m.State().Status != CDPStatusStreaming {
		select {
		case <-deadline:
			t.Fatalf("never reached Streaming; state=%v", m.State().Status)
		case <-time.After(20 * time.Millisecond):
		}
	}
	if got := string(m.LatestFrame()); got != string(frame) {
		t.Fatalf("LatestFrame = %q, want %q", got, frame)
	}
	var methods []string
	for len(methods) < 2 {
		select {
		case c := <-cmds:
			methods = append(methods, c)
		case <-time.After(time.Second):
			t.Fatalf("page endpoint saw only %v", methods)
		}
	}
	b, _ := json.Marshal(methods)
	if string(b) != `["Page.enable","Page.startScreencast"]` {
		t.Fatalf("page endpoint commands = %s", b)
	}
}
