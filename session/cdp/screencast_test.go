package cdp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
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

// fakePageChrome serves /json/list plus separate browser and page WebSocket endpoints.
// Like real Chrome, the browser endpoint rejects Page.* and Input.*.
type fakePageChrome struct {
	port int

	mu          sync.Mutex
	pageMethods []string
	browserMsgs []string

	// onStartScreencast runs on the page connection after Page.startScreencast is acked.
	onStartScreencast   func(conn *websocket.Conn)
	failStartScreencast bool
	noPage              bool
}

func newFakePageChrome(t *testing.T) *fakePageChrome {
	t.Helper()
	f := &fakePageChrome{}
	up := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/json/list", func(w http.ResponseWriter, r *http.Request) {
		targets := []map[string]string{}
		if !f.noPage {
			targets = append(targets, map[string]string{
				"type": "page", "webSocketDebuggerUrl": "ws://" + r.Host + "/devtools/page/P1",
			})
		}
		_ = json.NewEncoder(w).Encode(targets)
	})
	serve := func(page bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			conn, err := up.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			for {
				var msg cdpMessage
				if err := conn.ReadJSON(&msg); err != nil {
					return
				}
				f.mu.Lock()
				if page {
					f.pageMethods = append(f.pageMethods, msg.Method)
				} else {
					f.browserMsgs = append(f.browserMsgs, msg.Method)
				}
				f.mu.Unlock()
				if !page || (msg.Method == "Page.startScreencast" && f.failStartScreencast) {
					_ = conn.WriteJSON(cdpMessage{ID: msg.ID, Error: &cdpError{Code: -32601, Message: "'" + msg.Method + "' wasn't found"}})
					continue
				}
				_ = conn.WriteJSON(cdpMessage{ID: msg.ID, Result: json.RawMessage(`{}`)})
				if msg.Method == "Page.startScreencast" && f.onStartScreencast != nil {
					f.onStartScreencast(conn)
				}
			}
		}
	}
	mux.HandleFunc("/devtools/browser/B1", serve(false))
	mux.HandleFunc("/devtools/page/P1", serve(true))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	f.port, _ = strconv.Atoi(u.Port())
	return f
}

func (f *fakePageChrome) pageSeen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.pageMethods...)
}

func sendFrame(conn *websocket.Conn, data string) {
	params, _ := json.Marshal(map[string]any{"data": data, "sessionId": 1})
	_ = conn.WriteJSON(cdpMessage{Method: "Page.screencastFrame", Params: params})
}

// startScreencastAsync runs runScreencast against the fake's page endpoint.
func startScreencastAsync(t *testing.T, f *fakePageChrome) *cdpStreamManager {
	t.Helper()
	m := &cdpStreamManager{cfg: CDPConfig{SessionID: "t"}, state: CDPState{Status: CDPStatusNoBrowser, Port: f.port}}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	pageURL := fmt.Sprintf("ws://127.0.0.1:%d/devtools/page/P1", f.port)
	go func() { _ = m.runScreencast(ctx, pageURL, f.port) }()
	return m
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	require.Eventually(t, cond, 5*time.Second, 10*time.Millisecond, "timed out waiting for %s", what)
}

func TestRunScreencast_WaitsForFirstFrameThenDispatchesInputToPage(t *testing.T) {
	f := newFakePageChrome(t)
	release := make(chan struct{})
	f.onStartScreencast = func(conn *websocket.Conn) {
		<-release // handler goroutine is the only server-side writer
		sendFrame(conn, base64.StdEncoding.EncodeToString([]byte("jpeg-bytes")))
	}
	m := startScreencastAsync(t, f)

	waitFor(t, "Page.startScreencast on page endpoint", func() bool {
		return strings.Join(f.pageSeen(), ",") == "Page.enable,Page.startScreencast"
	})
	require.Equal(t, CDPStatusWaiting, m.State().Status, "before first frame")
	require.Nil(t, m.LatestFrame())

	close(release)
	waitFor(t, "Streaming", func() bool { return m.State().Status == CDPStatusStreaming })
	require.Equal(t, "jpeg-bytes", string(m.LatestFrame()))

	require.NoError(t, m.DispatchInput([]byte(`{"method":"Input.dispatchMouseEvent","params":{"type":"mouseMoved","x":1,"y":2}}`)))
	require.NoError(t, m.DispatchInput([]byte(`{"method":"Input.dispatchKeyEvent","params":{"type":"keyDown"}}`)))
	waitFor(t, "Input.* on page endpoint", func() bool {
		s := strings.Join(f.pageSeen(), ",")
		return strings.Contains(s, "Input.dispatchMouseEvent") && strings.Contains(s, "Input.dispatchKeyEvent")
	})

	f.mu.Lock()
	defer f.mu.Unlock()
	require.Empty(t, f.browserMsgs, "browser endpoint must receive nothing")
}

func TestRunScreencast_ErrorReply_NeverStreams(t *testing.T) {
	f := newFakePageChrome(t)
	f.failStartScreencast = true
	m := startScreencastAsync(t, f)

	waitFor(t, "Page.startScreencast", func() bool { return len(f.pageSeen()) == 2 })
	require.Never(t, func() bool { return m.State().Status == CDPStatusStreaming },
		100*time.Millisecond, 10*time.Millisecond, "error reply must not publish Streaming")
}

func TestPageWebSocketURL_NoPageTarget_Errors(t *testing.T) {
	f := newFakePageChrome(t)
	f.noPage = true
	_, err := pageWebSocketURL(context.Background(), f.port)
	require.Error(t, err)
}

func TestRunScreencast_InvalidFrame_StaysWaiting(t *testing.T) {
	f := newFakePageChrome(t)
	f.onStartScreencast = func(conn *websocket.Conn) { sendFrame(conn, "!!not-base64!!") }
	m := startScreencastAsync(t, f)

	waitFor(t, "Page.startScreencast", func() bool { return len(f.pageSeen()) == 2 })
	require.Never(t, func() bool {
		return m.State().Status != CDPStatusWaiting || m.LatestFrame() != nil
	}, 100*time.Millisecond, 10*time.Millisecond, "invalid frame must leave status Waiting with no frame")
}
