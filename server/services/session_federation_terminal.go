package services

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/protocol"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/streamhub"
)

const (
	// maxFederationTerminalFrame bounds one snapshot line a peer can make us buffer.
	maxFederationTerminalFrame = 8 << 20
	maxFederatedSessionIDLen   = 512
)

// ErrFederatedHostUnavailable means the owning host is not in the live registry
// or none of its addresses answered.
var ErrFederatedHostUnavailable = errors.New("federated host unavailable")

type terminalFrame struct {
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`
}

// SessionFederationTerminalHandler streams this host's pane snapshots for one
// session to an enrolled peer as JSON lines. Read-only by construction: the
// request body is never read, so a peer cannot inject input or resize.
func SessionFederationTerminalHandler(peers FederationPeers, capture func(sessionID string) (string, error), poll time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := peers.AuthorizePeerRequest(r.Header, r.Method, r.URL.Path, time.Now()); err != nil {
			log.Debug("session_federation.terminal_unauthorized", "err", err)
			http.Error(w, "session federation requires a signed request from an enrolled host", http.StatusUnauthorized)
			return
		}
		id := r.URL.Query().Get("session")
		if id == "" || len(id) > maxFederatedSessionIDLen {
			http.Error(w, "missing or oversized session id", http.StatusBadRequest)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		enc := json.NewEncoder(w)
		var last string
		first := true
		t := time.NewTicker(poll)
		defer t.Stop()
		for {
			content, err := capture(id)
			if err != nil {
				_ = enc.Encode(terminalFrame{Error: "session unavailable"})
				flusher.Flush()
				return
			}
			if first || content != last {
				if err := enc.Encode(terminalFrame{Content: content}); err != nil {
					return
				}
				flusher.Flush()
				last, first = content, false
			}
			select {
			case <-r.Context().Done():
				return
			case <-t.C:
			}
		}
	}
}

// StreamTerminal relays pane snapshots of a peer's session to send until ctx is
// done, the peer closes the stream, or send fails. Read-only: nothing is sent
// to the peer beyond the signed request.
func (f *FederatedSessions) StreamTerminal(ctx context.Context, id session.FederatedSessionID, send func(content string) error) error {
	var entry *session.RegistryEntry
	for _, e := range f.peers.LiveSnapshot() {
		if e.HostID.String() == id.HostID && e.HostID != f.identity.ID {
			e := e
			entry = &e
			break
		}
	}
	if entry == nil {
		return fmt.Errorf("%w: %s", ErrFederatedHostUnavailable, id.HostID)
	}
	var lastErr error
	for _, addr := range entry.AdvertisedAddress {
		connected, err := f.streamFrom(ctx, addr, id.SessionID, send)
		if connected || ctx.Err() != nil {
			return err
		}
		lastErr = err
	}
	return fmt.Errorf("%w: %s: %v", ErrFederatedHostUnavailable, id.HostID, lastErr)
}

// streamFrom reports connected=true once the peer accepted the stream, after
// which its error is final rather than a reason to try another address.
func (f *FederatedSessions) streamFrom(ctx context.Context, addr, sessionID string, send func(string) error) (connected bool, err error) {
	u := "https://" + addr + session.SessionFederationTerminalPath + "?session=" + url.QueryEscape(sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}
	session.SignPeerRequest(req.Header, f.identity, http.MethodGet, session.SessionFederationTerminalPath, f.now())
	resp, err := f.streamClient.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("session federation terminal at %s: status %d", addr, resp.StatusCode)
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), maxFederationTerminalFrame)
	for sc.Scan() {
		var fr terminalFrame
		if err := json.Unmarshal(sc.Bytes(), &fr); err != nil {
			return true, fmt.Errorf("decode terminal frame from %s: %w", addr, err)
		}
		if fr.Error != "" {
			return true, errors.New(fr.Error)
		}
		if err := send(fr.Content); err != nil {
			return true, err
		}
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		return true, err
	}
	return true, nil
}

// streamFederatedTerminal shows a peer's session in the browser's terminal,
// read-only: browser input and resize frames are discarded, never forwarded.
func (h *ConnectRPCWebSocketHandler) streamFederatedTerminal(stream *connectWebSocketStream, id session.FederatedSessionID, wireID string) error {
	fed := h.sessionService.federation
	if fed == nil {
		return fmt.Errorf("%w: federation is not enabled", ErrFederatedHostUnavailable)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Drain the browser so a disconnect is noticed; its frames are read-only-dropped.
	go func() {
		defer cancel()
		for {
			if _, _, err := stream.conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	return fed.StreamTerminal(ctx, id, func(content string) error {
		frame := newTerminalOutputData(wireID, ansiSnapshotPrefix+prepareSnapshotContent(streamhub.RawPaneContent(content)), false)
		b, err := proto.Marshal(frame)
		if err != nil {
			return err
		}
		return stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, b))
	})
}
