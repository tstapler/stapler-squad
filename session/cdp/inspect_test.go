package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeChrome serves /json/list and a page-target WebSocket answering CDP
// commands from the handlers map; unknown methods get an empty result.
// An "event" is also emitted before each response to prove events are skipped.
func fakeChrome(t *testing.T, handlers map[string]func(params map[string]any) (any, string)) int {
	t.Helper()
	up := websocket.Upgrader{}
	mux := http.NewServeMux()
	var srvURL string
	mux.HandleFunc("/json/list", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]string{
			{"type": "background_page", "webSocketDebuggerUrl": "ws://ignored"},
			{"type": "page", "webSocketDebuggerUrl": "ws" + strings.TrimPrefix(srvURL, "http") + "/page"},
		})
	})
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			var req struct {
				ID     int            `json:"id"`
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if err := c.ReadJSON(&req); err != nil {
				return
			}
			_ = c.WriteJSON(map[string]any{"method": "DOM.setChildNodes", "params": map[string]any{}})
			resp := map[string]any{"id": req.ID, "result": map[string]any{}}
			if h, ok := handlers[req.Method]; ok {
				result, errMsg := h(req.Params)
				if errMsg != "" {
					resp = map[string]any{"id": req.ID, "error": map[string]any{"message": errMsg}}
				} else {
					resp["result"] = result
				}
			}
			_ = c.WriteJSON(resp)
		}
	})
	srv := httptest.NewServer(mux)
	srvURL = srv.URL
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	return port
}

func okHandlers(html string, quad []float64) map[string]func(map[string]any) (any, string) {
	return map[string]func(map[string]any) (any, string){
		"DOM.getNodeForLocation": func(map[string]any) (any, string) {
			return map[string]any{"backendNodeId": 7}, ""
		},
		"DOM.pushNodesByBackendIdsToFrontend": func(map[string]any) (any, string) {
			return map[string]any{"nodeIds": []int{42}}, ""
		},
		"DOM.getBoxModel": func(map[string]any) (any, string) {
			return map[string]any{"model": map[string]any{"content": quad}}, ""
		},
		"DOM.getOuterHTML": func(map[string]any) (any, string) {
			return map[string]any{"outerHTML": html}, ""
		},
		"CSS.getComputedStyleForNode": func(map[string]any) (any, string) {
			return map[string]any{"computedStyle": []map[string]string{
				{"name": "zoom", "value": "1"},
				{"name": "color", "value": "red"},
				{"name": "display", "value": "block"},
			}}, ""
		},
		"Page.captureScreenshot": func(map[string]any) (any, string) {
			return map[string]any{"data": "SkZJRg=="}, ""
		},
	}
}

func TestInspectAtPoint_Success(t *testing.T) {
	port := fakeChrome(t, okHandlers("<b>hi</b>", []float64{10, 20, 110, 20, 110, 60, 10, 60}))
	res, err := InspectAtPoint(context.Background(), port, Point{X: 15, Y: 25})
	require.NoError(t, err)
	assert.Equal(t, "<b>hi</b>", res.OuterHTML)
	assert.Equal(t, "SkZJRg==", res.ScreenshotBase64)
	assert.Equal(t, BoundingBox{X: 10, Y: 20, Width: 100, Height: 40}, res.BoundingBox)
	require.GreaterOrEqual(t, len(res.ComputedCSS), 3)
	// Priority properties come first, ahead of "zoom".
	assert.Equal(t, "display", res.ComputedCSS[0].Name)
	assert.Equal(t, "color", res.ComputedCSS[1].Name)
	assert.Equal(t, "zoom", res.ComputedCSS[2].Name)
}

func TestInspectAtPoint_NoNode(t *testing.T) {
	h := okHandlers("", nil)
	h["DOM.getNodeForLocation"] = func(map[string]any) (any, string) { return nil, "No node found at given location" }
	_, err := InspectAtPoint(context.Background(), fakeChrome(t, h), Point{X: 1, Y: 1})
	assert.ErrorIs(t, err, ErrNoElementAtPoint)
}

func TestInspectAtPoint_ZeroAreaOmitsScreenshot(t *testing.T) {
	port := fakeChrome(t, okHandlers("<i/>", []float64{5, 5, 5, 5, 5, 5, 5, 5}))
	res, err := InspectAtPoint(context.Background(), port, Point{X: 5, Y: 5})
	require.NoError(t, err)
	assert.Empty(t, res.ScreenshotBase64)
}

func TestInspectAtPoint_ScreenshotFailureIsPartial(t *testing.T) {
	h := okHandlers("<p/>", []float64{0, 0, 10, 0, 10, 10, 0, 10})
	h["Page.captureScreenshot"] = func(map[string]any) (any, string) { return nil, "boom" }
	res, err := InspectAtPoint(context.Background(), fakeChrome(t, h), Point{X: 1, Y: 1})
	require.NoError(t, err)
	assert.Empty(t, res.ScreenshotBase64)
	assert.Equal(t, "<p/>", res.OuterHTML)
}

func TestInspectAtPoint_TruncatesHTML(t *testing.T) {
	big := "<div>" + strings.Repeat("é", MaxInspectHTMLBytes) + "</div>"
	res, err := InspectAtPoint(context.Background(),
		fakeChrome(t, okHandlers(big, []float64{0, 0, 1, 0, 1, 1, 0, 1})), Point{X: 1, Y: 1})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(res.OuterHTML), MaxInspectHTMLBytes)
	assert.True(t, strings.HasSuffix(res.OuterHTML, htmlTruncationMark))
	assert.True(t, json.Valid([]byte(strconv.Quote(res.OuterHTML))), "must remain valid UTF-8")
}

func TestInspectAtPoint_CancelledContext(t *testing.T) {
	port := fakeChrome(t, okHandlers("", nil))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := InspectAtPoint(ctx, port, Point{X: 1, Y: 1})
	assert.Error(t, err)
}

func TestInspectAtPoint_NoChrome(t *testing.T) {
	_, err := InspectAtPoint(context.Background(), 1, Point{X: 1, Y: 1})
	assert.Error(t, err)
}

func TestSelectCSS_CapAndPriority(t *testing.T) {
	var all []CSSProperty
	for i := 0; i < 100; i++ {
		all = append(all, CSSProperty{Name: fmt.Sprintf("prop-%d", i), Value: "v"})
	}
	all = append(all, CSSProperty{Name: "margin-top", Value: "4px"}, CSSProperty{Name: "display", Value: "flex"})
	out := selectCSS(all)
	assert.Len(t, out, MaxInspectCSSProps)
	assert.Equal(t, "display", out[0].Name)
	assert.Equal(t, "margin-top", out[1].Name, "shorthand falls back to its longhands")
}

func TestInspectAtPoint_ScreenshotClipIncludesScrollOffset(t *testing.T) {
	h := okHandlers("<p/>", []float64{10, 20, 110, 20, 110, 60, 10, 60})
	h["Page.getLayoutMetrics"] = func(map[string]any) (any, string) {
		return map[string]any{"cssVisualViewport": map[string]any{"pageX": 5, "pageY": 300}}, ""
	}
	var clip map[string]any
	h["Page.captureScreenshot"] = func(p map[string]any) (any, string) {
		clip, _ = p["clip"].(map[string]any)
		return map[string]any{"data": "QQ=="}, ""
	}
	_, err := InspectAtPoint(context.Background(), fakeChrome(t, h), Point{X: 15, Y: 25})
	require.NoError(t, err)
	assert.EqualValues(t, 15, clip["x"])
	assert.EqualValues(t, 320, clip["y"])
}

func TestBoxFor_RotatedQuadUsesBounds(t *testing.T) {
	h := okHandlers("", []float64{50, 0, 100, 50, 50, 100, 0, 50})
	port := fakeChrome(t, h)
	res, err := InspectAtPoint(context.Background(), port, Point{X: 1, Y: 1})
	require.NoError(t, err)
	assert.Equal(t, BoundingBox{X: 0, Y: 0, Width: 100, Height: 100}, res.BoundingBox)
}
