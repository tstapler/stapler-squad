package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/cdp"
)

func inspectReq(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/sessions/s1/inspect-element", strings.NewReader(body))
	r.SetPathValue("id", "s1")
	return r
}

func TestInspectElementHandler_CDPPortZero_Returns503JSON(t *testing.T) {
	t.Parallel()
	// A fresh test instance has a no-op CDP manager whose Port() is 0.
	h := NewInspectElementHandler(&fakeInstanceFinder{inst: newTestInstance(t)})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, inspectReq(`{"x":1,"y":2}`))

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body["error"], "CDP")
}

func TestInspectElementHandler_SessionNotFound_Returns404(t *testing.T) {
	t.Parallel()
	h := NewInspectElementHandler(&fakeInstanceFinder{})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, inspectReq(`{"x":1,"y":2}`))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestInspectElementHandler_BadBody_Returns400(t *testing.T) {
	t.Parallel()
	h := NewInspectElementHandler(&fakeInstanceFinder{inst: newTestInstance(t)})
	for _, body := range []string{`not json`, `{}`, `{"x":1}`} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, inspectReq(body))
		assert.Equal(t, http.StatusBadRequest, w.Code, body)
	}
}

func stubbedInspectHandler(t *testing.T, fn func(context.Context, int, cdp.Point) (*cdp.InspectResult, error)) *InspectElementHandler {
	h := NewInspectElementHandler(&fakeInstanceFinder{inst: newTestInstance(t)})
	h.portOf = func(*session.Instance) int { return 9222 }
	h.inspect = fn
	return h
}

func TestInspectElementHandler_Success(t *testing.T) {
	t.Parallel()
	var gotPort int
	var gotPt cdp.Point
	h := stubbedInspectHandler(t, func(_ context.Context, port int, pt cdp.Point) (*cdp.InspectResult, error) {
		gotPort, gotPt = port, pt
		return &cdp.InspectResult{OuterHTML: "<b/>", ScreenshotBase64: "abc"}, nil
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, inspectReq(`{"x":0,"y":7}`))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 9222, gotPort)
	assert.Equal(t, cdp.Point{X: 0, Y: 7}, gotPt, "zero coordinates are valid")
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "<b/>", body["html"])
	assert.Equal(t, "abc", body["screenshotBase64"])
}

func TestInspectElementHandler_NoElement_Returns200NotFound(t *testing.T) {
	t.Parallel()
	h := stubbedInspectHandler(t, func(context.Context, int, cdp.Point) (*cdp.InspectResult, error) {
		return nil, cdp.ErrNoElementAtPoint
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, inspectReq(`{"x":1,"y":2}`))
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"html":"","notFound":true}`, w.Body.String())
}

func TestInspectElementHandler_CDPError_Returns502(t *testing.T) {
	t.Parallel()
	h := stubbedInspectHandler(t, func(context.Context, int, cdp.Point) (*cdp.InspectResult, error) {
		return nil, errors.New("dial refused")
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, inspectReq(`{"x":1,"y":2}`))
	assert.Equal(t, http.StatusBadGateway, w.Code)
	assert.NotContains(t, w.Body.String(), "dial refused", "internal errors are not leaked")
}

func TestCDPPortOf_NilManager_ReturnsZero(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, cdpPortOf(&session.Instance{}))
}
