package services

// inspect_element_handler.go implements POST /api/sessions/{id}/inspect-element:
// given viewport coordinates it asks the session's Chrome (over CDP) for the
// element's HTML, computed CSS and a cropped screenshot.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/cdp"
)

const maxInspectBodyBytes = 1024

// InspectElementHandler serves element-inspection requests for Browser Passthrough.
type InspectElementHandler struct {
	finder  InstanceFinder
	inspect func(ctx context.Context, port int, pt cdp.Point) (*cdp.InspectResult, error)
	portOf  func(inst *session.Instance) int
}

// NewInspectElementHandler creates a handler backed by the given InstanceFinder.
func NewInspectElementHandler(finder InstanceFinder) *InspectElementHandler {
	return &InspectElementHandler{
		finder:  finder,
		inspect: cdp.InspectAtPoint,
		portOf:  func(inst *session.Instance) int { return inst.CDPManager().Port() },
	}
}

type inspectRequest struct {
	X *int `json:"x"`
	Y *int `json:"y"`
}

// inspectNotFoundResponse is returned with 200 when no element is at the point.
type inspectNotFoundResponse struct {
	HTML     string `json:"html"`
	NotFound bool   `json:"notFound"`
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// +http: POST /api/sessions/{id}/inspect-element browser:inspect-element
// ServeHTTP handles an element-inspection request.
func (h *InspectElementHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing session id")
		return
	}
	var req inspectRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxInspectBodyBytes)).Decode(&req); err != nil || req.X == nil || req.Y == nil {
		writeJSONError(w, http.StatusBadRequest, "body must be JSON {\"x\": int, \"y\": int}")
		return
	}
	inst := h.finder.FindInstance(sessionID)
	if inst == nil {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return
	}
	port := h.portOf(inst)
	if port == 0 {
		writeJSONError(w, http.StatusServiceUnavailable, "CDP not connected: Chrome is not running or CDP is unavailable for this session")
		return
	}

	res, err := h.inspect(r.Context(), port, cdp.Point{X: *req.X, Y: *req.Y})
	w.Header().Set("Content-Type", "application/json")
	switch {
	case errors.Is(err, cdp.ErrNoElementAtPoint):
		_ = json.NewEncoder(w).Encode(inspectNotFoundResponse{NotFound: true})
	case err != nil:
		log.Warn("inspect-element: CDP inspection failed", "session", sessionID, "err", err)
		writeJSONError(w, http.StatusBadGateway, "element inspection failed")
	default:
		_ = json.NewEncoder(w).Encode(res)
	}
}
