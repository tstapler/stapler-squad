package services

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git"
)

const sessionIDHeader = "X-CS-Session-ID"

// HookReceiver handles inbound Claude Code hook callbacks for non-approval events.
// These are fire-and-forget: Claude does not block on the response.
type HookReceiver struct {
	// driftMu guards driftLastChecked, the per-worktree rate limiter used by
	// HandlePostToolUseDriftCheck (see hook_receiver_drift.go).
	driftMu          sync.Mutex
	driftLastChecked map[string]time.Time

	// driftCheckMinInterval overrides defaultDriftCheckMinInterval; zero means "use
	// the default". Overridable via SetDriftCheckMinInterval for tests.
	driftCheckMinInterval time.Duration
	// driftThreshold overrides git.SteeringBranchDriftThreshold; zero means "use the
	// default". Overridable via SetDriftThreshold for tests.
	driftThreshold int
	// driftCheckFn computes how many commits worktreePath is behind origin/mainBranch.
	// Defaults to git.BehindOriginMain; overridable via SetDriftCheckFn so tests can
	// exercise HandlePostToolUseDriftCheck without a real git repo/network fetch.
	driftCheckFn func(worktreePath, mainBranch string) (int, error)

	// postToolUse, when set, hears every PostToolUse receipt with the tool name.
	postToolUse PostToolUseObserver

	// stop, when set, hears every Stop hook with its parsed body.
	stop StopObserver
}

// PostToolUseObserver is told which tool a session's PostToolUse hook ran for.
// sessionID is the X-CS-Session-ID header value (the session title).
type PostToolUseObserver func(ctx context.Context, sessionID, toolName string)

// SetPostToolUseObserver installs the receipt observer; call it before serving.
func (h *HookReceiver) SetPostToolUseObserver(o PostToolUseObserver) { h.postToolUse = o }

// SetStopObserver installs the Stop observer; call it before serving.
func (h *HookReceiver) SetStopObserver(o StopObserver) { h.stop = o }

// NewHookReceiver creates a HookReceiver.
func NewHookReceiver() *HookReceiver {
	return &HookReceiver{
		driftLastChecked: make(map[string]time.Time),
		driftCheckFn:     git.BehindOriginMain,
	}
}

// RegisterRoutes registers the non-approval hook endpoints on mux.
func (h *HookReceiver) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/hooks/stop", h.HandleStop)
	mux.HandleFunc("/api/hooks/pre-tool-use", h.HandlePreToolUse)
	mux.HandleFunc("/api/hooks/post-tool-use", h.HandlePostToolUse)
	mux.HandleFunc("/api/hooks/prompt-submit", h.HandlePromptSubmit)
	mux.HandleFunc("/api/hooks/post-tool-use-drift-check", h.HandlePostToolUseDriftCheck)
}

// HandleStop receives the Claude Code Stop hook.
func (h *HookReceiver) HandleStop(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get(sessionIDHeader)
	body, size := h.readBody(r)
	log.Info("[hook/stop]", "session", sessionID, "bytes", size)
	if h.stop != nil {
		payload := parseStopPayload(body)
		payload.ReceivedAt = time.Now()
		h.stop(r.Context(), sessionID, payload)
	}
	w.WriteHeader(http.StatusOK)
}

// HandlePreToolUse receives the Claude Code PreToolUse hook.
func (h *HookReceiver) HandlePreToolUse(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get(sessionIDHeader)
	tool := r.Header.Get("X-CS-Tool-Name")
	size := h.drainBody(r)
	log.Info("[hook/pre-tool-use]", "session", sessionID, "tool", tool, "bytes", size)
	w.WriteHeader(http.StatusOK)
}

// HandlePostToolUse receives the Claude Code PostToolUse hook.
func (h *HookReceiver) HandlePostToolUse(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get(sessionIDHeader)
	tool := r.Header.Get("X-CS-Tool-Name")
	body, size := h.readBody(r)
	if tool == "" {
		tool = toolNameOf(body) // the generated hook command sets no header; the name is only in the body
	}
	log.Info("[hook/post-tool-use]", "session", sessionID, "tool", tool, "bytes", size)
	if h.postToolUse != nil {
		h.postToolUse(r.Context(), sessionID, tool)
	}
	w.WriteHeader(http.StatusOK)
}

// HandlePromptSubmit receives the Claude Code UserPromptSubmit hook.
func (h *HookReceiver) HandlePromptSubmit(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get(sessionIDHeader)
	size := h.drainBody(r)
	log.Info("[hook/prompt-submit]", "session", sessionID, "bytes", size)
	w.WriteHeader(http.StatusOK)
}

// maxHookBody bounds how much of a hook payload is read.
const maxHookBody = 64 * 1024

// readBody returns the (bounded) body and its length. The body may hold
// secrets, so callers extract fields and never log it.
func (h *HookReceiver) readBody(r *http.Request) ([]byte, int64) {
	b, _ := io.ReadAll(io.LimitReader(r.Body, maxHookBody))
	return b, int64(len(b))
}

// parseStopPayload decodes a Stop body; a malformed one yields the zero payload.
func parseStopPayload(body []byte) StopPayload {
	var p StopPayload
	_ = json.Unmarshal(body, &p)
	return p
}

// toolNameOf extracts tool_name from a hook payload, or "".
func toolNameOf(body []byte) string {
	var p struct {
		ToolName string `json:"tool_name"`
	}
	if json.Unmarshal(body, &p) != nil {
		return ""
	}
	return p.ToolName
}

// drainBody discards the request body and returns the number of bytes read.
// Payloads may contain secrets; we log only the size for observability.
func (h *HookReceiver) drainBody(r *http.Request) int64 {
	n, _ := io.Copy(io.Discard, io.LimitReader(r.Body, 64*1024))
	return n
}
