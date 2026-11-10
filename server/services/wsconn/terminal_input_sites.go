package wsconn

import (
	"net/http"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services"
)

// Wiring of terminal input observability into the WebSocket input sites. Kept out of
// connectrpc_websocket.go so the already-complex read loops gain no complexity.

// listenerNamer is implemented by stream conns that know their listener (a mux stream).
type listenerNamer interface{ listenerName() string }

// listenerName reports the listener of the mux socket carrying this stream.
func (s *muxStreamConn) listenerName() string {
	if s.sock == nil {
		return ""
	}
	return s.sock.listener
}

// inputSource returns the stream's input source for path, building it on first use so
// log throttling is per stream. A nil stream yields nil, which records nothing.
func (s *connectWebSocketStream) inputSource(path services.InputPath, sessionID string) *services.InputSource {
	if s == nil {
		return nil
	}
	s.inputOnce.Do(func() {
		listener := s.listener
		if n, ok := s.conn.(listenerNamer); listener == "" && ok {
			listener = n.listenerName()
		}
		s.input = services.NewInputSource(path, listener, sessionID)
	})
	return s.input
}

// handleReadLoopInput counts one frame from the shared read loop (path "cm") and forwards
// it when a writer is attached. forwarded is recorded only if onInput reports it issued
// the write, so a permission denial is visible as received without forwarded.
func handleReadLoopInput(p inputReadLoopParams, writer services.TerminalWriter, data []byte) {
	src := p.stream.inputSource(services.PathCM, p.sessionID)
	services.RecordTerminalInput(src, data, services.StageReceived)
	if writer == nil {
		log.Debug("[runInputReadLoop] read-only attach: input frame dropped", "session", p.sessionID, "bytes", len(data))
		return
	}
	if p.onInput(data) {
		services.RecordTerminalInput(src, data, services.StageForwarded)
	}
}

// requestListener names the listener that accepted r ("" when no chain stamped it).
func requestListener(r *http.Request) string {
	rec, _ := services.RequestRecordFrom(r.Context())
	return rec.Listener
}
