package services

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/protocol"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// inputRow returns the default tally's row for a session (zero row when absent).
func inputRow(id string) TerminalInputSession {
	for _, row := range TerminalInputReport() {
		if row.SessionID == id {
			return row
		}
	}
	return TerminalInputSession{}
}

func assertPathCounts(t *testing.T, id string, path InputPath, received, forwarded int64) {
	t.Helper()
	assert.Equal(t, TerminalInputPathCounts{Received: received, Forwarded: forwarded}, inputRow(id).ByPath[path.String()])
}

func newSiteSessionID(prefix string) string { return prefix + "-" + uuid.NewString()[:8] }

// Shared read loop (path cm; also covers the :1653 and :2489 onInput closures, which return
// the forwarded result this dispatch records).
func TestInputSites_ShouldRecordReceivedAndForwarded_When_PermittedWriterAtSharedReadLoop(t *testing.T) {
	t.Parallel()
	id := newSiteSessionID("site-cm")
	inst := hiddenInstance(t, false)
	p := inputReadLoopParams{
		writer:    writerForHandler(t, &fakeTmuxSender{}, inst),
		stream:    &connectWebSocketStream{listener: ListenerRemote},
		sessionID: id,
		onInput:   func([]byte) bool { return true },
	}
	require.NoError(t, dispatchInputReadLoopFrame(p, inputFrame("ls\r")))
	assertPathCounts(t, id, PathCM, 1, 1)
}

func TestInputSites_ShouldRecordReceivedWithoutForwarded_When_ReadOnlyOrCanSendCommandFalse(t *testing.T) {
	t.Parallel()
	t.Run("read-only subscriber", func(t *testing.T) {
		t.Parallel()
		id := newSiteSessionID("site-cm-ro")
		called := false
		p := inputReadLoopParams{
			stream:    &connectWebSocketStream{listener: ListenerRemote},
			sessionID: id,
			onInput:   func([]byte) bool { called = true; return true },
		}
		require.NoError(t, dispatchInputReadLoopFrame(p, inputFrame("x")))
		assert.False(t, called)
		assertPathCounts(t, id, PathCM, 1, 0)
		assert.EqualValues(t, map[string]int64{"text": 1}, inputRow(id).ByKind)
	})
	t.Run("writer attached but CanSendCommand false", func(t *testing.T) {
		t.Parallel()
		id := newSiteSessionID("site-cm-deny")
		inst := hiddenInstance(t, false)
		p := inputReadLoopParams{
			writer:    writerForHandler(t, &fakeTmuxSender{}, inst),
			stream:    &connectWebSocketStream{listener: ListenerRemote},
			sessionID: id,
			onInput:   func([]byte) bool { return false }, // what both closures return on denial
		}
		require.NoError(t, dispatchInputReadLoopFrame(p, inputFrame("x")))
		assertPathCounts(t, id, PathCM, 1, 0)
	})
}

func TestInputSites_ShouldRecordReceivedAndForwarded_When_PermittedWriterAtCaptureSite(t *testing.T) {
	t.Parallel()
	id := newSiteSessionID("site-capture")
	inst := hiddenInstance(t, false)
	sender := &fakeTmuxSender{}
	p := capturePaneParamsFor(inst, writerForHandler(t, sender, inst), false)
	p.cpt.sessionID = id
	p.stream = &connectWebSocketStream{listener: ListenerLocal}

	handleCapturePaneInput(p, []byte("ls\r"))
	assertPathCounts(t, id, PathCapture, 1, 1)

	denied := *p.snap
	denied.Permissions.CanSendCommand = false
	p.snap = &denied
	handleCapturePaneInput(p, []byte("ls\r"))
	assertPathCounts(t, id, PathCapture, 2, 1)
	inputs, _ := sender.counts()
	assert.Equal(t, 1, inputs)
}

func TestInputSites_ShouldRecordReceivedWithoutForwarded_When_HiddenSessionAtCaptureSite(t *testing.T) {
	t.Parallel()
	id := newSiteSessionID("site-capture-ro")
	inst := hiddenInstance(t, true)
	p := capturePaneParamsFor(inst, writerForHandler(t, &fakeTmuxSender{}, inst), false)
	p.cpt.sessionID = id
	p.stream = &connectWebSocketStream{listener: ListenerLocal}
	handleCapturePaneInput(p, []byte("x"))
	assertPathCounts(t, id, PathCapture, 1, 0)
}

func shellParamsFor(t *testing.T, id string, hidden, canSend bool) shellStreamParams {
	t.Helper()
	inst := hiddenInstance(t, hidden)
	inst.Permissions.CanSendCommand = canSend
	return shellStreamParams{
		writer:               writerForHandler(t, &fakeTmuxSender{}, inst),
		stream:               &connectWebSocketStream{listener: ListenerLocal},
		instance:             inst,
		shellSess:            tmux.NewTmuxSessionFromExisting("obs-shell-missing"),
		sessionID:            id,
		shellID:              "shell-1",
		shellTmuxSessionName: "obs-shell-missing",
		resizeCh:             make(chan shellResizeReq, 1),
	}
}

func TestInputSites_ShouldRecordReceivedAndForwarded_When_PermittedWriterAtShellSite(t *testing.T) {
	t.Parallel()
	id := newSiteSessionID("site-shell")
	handleShellInput(shellParamsFor(t, id, false, true), []byte("ls\r"))
	assertPathCounts(t, id, PathShell, 1, 1)
}

func TestInputSites_ShouldRecordReceivedWithoutForwarded_When_DeniedOrHiddenAtShellSite(t *testing.T) {
	t.Parallel()
	denied := newSiteSessionID("site-shell-deny")
	handleShellInput(shellParamsFor(t, denied, false, false), []byte("x"))
	assertPathCounts(t, denied, PathShell, 1, 0)

	hidden := newSiteSessionID("site-shell-ro")
	handleShellInput(shellParamsFor(t, hidden, true, true), []byte("x"))
	assertPathCounts(t, hidden, PathShell, 1, 0)
}

// session_service_stream_terminal.go: needs a real tmux-backed session, like the read-only test next to it.
func TestInputSites_ShouldRecordReceivedAndForwarded_When_PermittedWriterAtStreamSite(t *testing.T) {
	svc, srv := newBidiStreamTestServer(t)
	poller := session.NewReviewQueuePoller(session.NewReviewQueue(), session.NewInstanceStatusManager(), nil)
	svc.SetReviewQueuePoller(poller)
	svc.SetRegistry(session.NewRegistry(nil, svc.WireInstanceCallbacks))
	client := sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL)

	spawn := func(hidden bool) *session.Instance {
		inst, err := svc.CreateDirectorySession(context.Background(), t.TempDir(), SessionSpawnOptions{
			Title: newSiteSessionID("obs-stream"), Hidden: hidden, ProgramOverride: "bash",
		})
		if err != nil {
			t.Skipf("tmux-backed session unavailable: %v", err)
		}
		t.Cleanup(func() { destroyCreatedSession(t, svc, inst.Snapshot().Title) })
		_ = wait.WaitForCondition(inst.Started, wait.WaitConfig{Timeout: 60 * time.Second, PollInterval: 100 * time.Millisecond, Description: "session started"})
		if !inst.Started() {
			t.Skip("session never started; tmux unavailable")
		}
		return inst
	}

	visible := spawn(false).Snapshot().Title
	typeThroughStreamTerminal(t, client, visible, "OBS_$((1+1))")
	assertPathCounts(t, visible, PathStream, 1, 1)

	hidden := spawn(true).Snapshot().Title
	typeThroughStreamTerminal(t, client, hidden, "OBS_$((1+1))")
	assertPathCounts(t, hidden, PathStream, 1, 0)
}

func inputEnvelope(t *testing.T, data string) []byte {
	t.Helper()
	b, err := proto.Marshal(inputFrame(data))
	require.NoError(t, err)
	return protocol.CreateEnvelope(0, b)
}

// /api/ws/terminal-mux: a stream on the mux socket reaches the shared read loop, which is a counted site.
func TestMuxInput_ShouldIncrementTally_When_TerminalMuxClientSendsInput(t *testing.T) {
	t.Parallel()
	id := newSiteSessionID("mux-tally")
	served := make(chan *inputSource, 1)
	serve := func(conn streamConn, _ string) {
		stream := &connectWebSocketStream{conn: conn}
		errChan := make(chan error, 1)
		readOneInputFrame(inputReadLoopParams{stream: stream, errChan: errChan, sessionID: id, writer: writerForHandler(t, &fakeTmuxSender{}, hiddenInstance(t, false)), onInput: func([]byte) bool { return true }})
		served <- stream.inputSource(PathCM, id)
	}
	c, _ := newMuxTestClient(t, serve, 1)
	c.send(encodeMuxEmpty(muxFrameOpen, 1))
	c.send(encodeMuxData(1, 2, true, inputEnvelope(t, "ls\r")))

	select {
	case src := <-served:
		assert.Equal(t, inputListenerLocal, src.listener)
	case <-time.After(muxTestWait):
		t.Fatal("mux stream never served the input frame")
	}
	assertPathCounts(t, id, PathCM, 1, 1)
	assert.EqualValues(t, 1, inputRow(id).Frames)
}

func TestInputSites_ShouldRecordListenerRemote_When_MuxAndRemoteListenerPaths(t *testing.T) {
	t.Parallel()
	t.Run("mux socket on the remote listener", func(t *testing.T) {
		t.Parallel()
		sock := newMuxSocket(NewTerminalMuxBridge(echoServe, 1), nil, httptest.NewRequest(http.MethodGet, "/", nil), ListenerRemote)
		stream := &connectWebSocketStream{conn: newMuxStreamConn(1, sock)}
		assert.Equal(t, inputListenerRemote, stream.inputSource(PathCM, "mux-remote").listener)
	})
	t.Run("direct websocket on the remote listener", func(t *testing.T) {
		t.Parallel()
		var got string
		h := WithRequestRecord(ListenerRemote, true)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = requestListener(r) }))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, ListenerRemote, got)
		stream := &connectWebSocketStream{listener: got}
		assert.Equal(t, inputListenerRemote, stream.inputSource(PathCM, "ws-remote").listener)
	})
}

func TestInputSites_ShouldExposeRegisteredCounters_When_MetricsGathered(t *testing.T) {
	t.Parallel()
	id := newSiteSessionID("site-gather")
	p := inputReadLoopParams{
		writer:    writerForHandler(t, &fakeTmuxSender{}, hiddenInstance(t, false)),
		stream:    &connectWebSocketStream{listener: ListenerRemote},
		sessionID: id,
		onInput:   func([]byte) bool { return true },
	}
	require.NoError(t, dispatchInputReadLoopFrame(p, inputFrame("x")))
	assert.NotNil(t, collectMetric(t, "terminal_input_frames_total"))
	assert.NotNil(t, collectMetric(t, "terminal_input_bytes_total"))
}

// inputSiteAllowList names every function that reads input frames or builds the shared
// read loop's params. A new entry point must be listed here, which means wiring it to
// recordTerminalInput first. Keys are "<file>:<function>".
var inputSiteAllowList = map[string]string{
	"connectrpc_websocket.go:dispatchInputReadLoopFrame": "shared read loop; handleReadLoopInput records received/forwarded (path cm)",
	"connectrpc_websocket.go:readOneCapturePaneFrame":    "capture-pane; handleCapturePaneInput records (path capture)",
	"connectrpc_websocket_shell.go:readOneShellFrame":    "shell tab; handleShellInput records (path shell)",
	"session_service_stream_terminal.go:StreamTerminal":  "Connect bidi stream; records inline (path stream)",
	"connectrpc_websocket.go:streamViaControlMode":       "builds inputReadLoopParams with onInput (path cm)",
	"connectrpc_websocket.go:streamViaHub":               "builds inputReadLoopParams with onInput (path cm)",
}

// scanInputSites finds, in non-test sources, every inputReadLoopParams literal (which must set
// onInput), every GetInput() call and every TerminalData_Input type-switch case.
func scanInputSites(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	found := map[string]bool{}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, f, nil, 0)
		require.NoError(t, err)
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if funcHasInputSite(t, fn) {
				found[f+":"+fn.Name.Name] = true
			}
		}
	}
	return found
}

func funcHasInputSite(t *testing.T, fn *ast.FuncDecl) bool {
	t.Helper()
	hit := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CompositeLit:
			if id, ok := x.Type.(*ast.Ident); ok && id.Name == "inputReadLoopParams" {
				hit = true
				assert.True(t, litSetsKey(x, "onInput"), "%s builds inputReadLoopParams without onInput", fn.Name.Name)
			}
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "GetInput" {
				hit = true
			}
		case *ast.CaseClause:
			for _, e := range x.List {
				if star, ok := e.(*ast.StarExpr); ok {
					if sel, ok := star.X.(*ast.SelectorExpr); ok && sel.Sel.Name == "TerminalData_Input" {
						hit = true
					}
				}
			}
		}
		return true
	})
	return hit
}

func litSetsKey(lit *ast.CompositeLit, key string) bool {
	for _, el := range lit.Elts {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			if id, ok := kv.Key.(*ast.Ident); ok && id.Name == key {
				return true
			}
		}
	}
	return false
}

func TestInputSites_ShouldFail_When_NewInputReadLoopParamsWiringNotInAllowList(t *testing.T) {
	t.Parallel()
	found := scanInputSites(t)
	var unlisted, stale []string
	for site := range found {
		if _, ok := inputSiteAllowList[site]; !ok {
			unlisted = append(unlisted, site)
		}
	}
	for site := range inputSiteAllowList {
		if !found[site] {
			stale = append(stale, site)
		}
	}
	sort.Strings(unlisted)
	sort.Strings(stale)
	assert.Empty(t, unlisted, "new input entry point: wire it to recordTerminalInput (received before gates, forwarded after the write) and add it to inputSiteAllowList")
	assert.Empty(t, stale, "allow-list entry no longer matches an input site; remove it")
}
