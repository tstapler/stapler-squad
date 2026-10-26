package cdp

// inspect.go implements one-shot element inspection over a short-lived CDP
// WebSocket, independent of the long-lived screencast connection in manager.go.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// MaxInspectHTMLBytes caps the outerHTML returned from an inspection.
	MaxInspectHTMLBytes = 8 * 1024
	// MaxInspectCSSProps caps the number of computed CSS properties returned.
	MaxInspectCSSProps = 50
	htmlTruncationMark = "\n<!-- truncated -->"
	inspectStepTimeout = time.Second
	// Screenshot payloads are base64 JPEGs; this bounds a hostile/huge outerHTML too.
	maxInspectMessageBytes = 16 << 20
)

// ErrNoElementAtPoint means no DOM node exists at the requested coordinates.
var ErrNoElementAtPoint = errors.New("cdp: no element at point")

// priorityCSSProps always appear first (in this order) and are never dropped.
var priorityCSSProps = []string{
	"display", "position", "width", "height", "color", "background-color",
	"font-size", "font-family", "border", "margin", "padding",
}

// CSSProperty is one computed CSS name/value pair.
type CSSProperty struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// BoundingBox is an element's content box in CSS pixels.
type BoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// InspectResult is the payload captured for the element under a point.
// ScreenshotBase64 is empty when the element has no area or the capture failed.
type InspectResult struct {
	OuterHTML        string        `json:"html"`
	ComputedCSS      []CSSProperty `json:"computedCss"`
	ScreenshotBase64 string        `json:"screenshotBase64"`
	BoundingBox      BoundingBox   `json:"boundingBox"`
}

type inspectConn struct {
	conn   *websocket.Conn
	nextID int
}

// call sends a command and reads until its response arrives, discarding events
// (DOM.enable floods the socket with DOM.* notifications).
func (c *inspectConn) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.nextID++
	id := c.nextID
	stepCtx, cancel := context.WithTimeout(ctx, inspectStepTimeout)
	defer cancel()
	deadline, _ := stepCtx.Deadline()
	_ = c.conn.SetWriteDeadline(deadline)
	_ = c.conn.SetReadDeadline(deadline)

	if err := c.conn.WriteJSON(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return nil, fmt.Errorf("cdp: send %s: %w", method, err)
	}
	for {
		if err := stepCtx.Err(); err != nil {
			return nil, fmt.Errorf("cdp: %s: %w", method, err)
		}
		var msg struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := c.conn.ReadJSON(&msg); err != nil {
			return nil, fmt.Errorf("cdp: read %s: %w", method, err)
		}
		if msg.ID != id {
			continue
		}
		if msg.Error != nil {
			return nil, fmt.Errorf("cdp: %s: %s", method, msg.Error.Message)
		}
		return msg.Result, nil
	}
}

// pageWebSocketURL returns the debugger URL of the first page target; the
// browser-level endpoint does not serve DOM/CSS domains.
func pageWebSocketURL(ctx context.Context, port int) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/json/list", port), nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("cdp: list targets: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cdp: list targets: status %d", resp.StatusCode)
	}
	var targets []struct {
		Type                 string `json:"type"`
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return "", fmt.Errorf("cdp: decode targets: %w", err)
	}
	for _, t := range targets {
		if t.Type == "page" && t.WebSocketDebuggerURL != "" {
			return t.WebSocketDebuggerURL, nil
		}
	}
	return "", errors.New("cdp: no page target")
}

// InspectAtPoint captures the element at pt (viewport CSS pixels) from the
// Chrome listening on port. Failures after the node lookup (HTML, CSS,
// screenshot) degrade to a partial result rather than an error.
func InspectAtPoint(ctx context.Context, port int, pt Point) (*InspectResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	wsURL, err := pageWebSocketURL(ctx, port)
	if err != nil {
		return nil, err
	}
	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	ws, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("cdp: dial page target: %w", err)
	}
	defer ws.Close()
	ws.SetReadLimit(maxInspectMessageBytes)
	c := &inspectConn{conn: ws}

	nodeID, err := resolveNodeAt(ctx, c, pt)
	if err != nil {
		return nil, err
	}

	res := &InspectResult{ComputedCSS: []CSSProperty{}}
	res.BoundingBox = boxFor(ctx, c, nodeID)
	res.OuterHTML = outerHTMLFor(ctx, c, nodeID)
	res.ComputedCSS = computedCSSFor(ctx, c, nodeID)
	if res.BoundingBox.Width > 0 && res.BoundingBox.Height > 0 {
		res.ScreenshotBase64 = screenshotFor(ctx, c, res.BoundingBox)
	}
	return res, nil
}

// Point is a viewport coordinate in CSS pixels.
type Point struct{ X, Y int }

// resolveNodeAt enables the needed domains and returns the frontend nodeId at pt.
func resolveNodeAt(ctx context.Context, c *inspectConn, pt Point) (int, error) {
	for _, m := range []string{"DOM.enable", "CSS.enable"} {
		if _, err := c.call(ctx, m, nil); err != nil {
			return 0, fmt.Errorf("cdp: resolve node: %w", err)
		}
	}
	// A document must be requested before node ids can be resolved.
	if _, err := c.call(ctx, "DOM.getDocument", map[string]any{"depth": 0}); err != nil {
		return 0, fmt.Errorf("cdp: resolve node: %w", err)
	}
	loc, err := c.call(ctx, "DOM.getNodeForLocation", map[string]any{
		"x": pt.X, "y": pt.Y, "includeUserAgentShadowDOM": false,
	})
	if err != nil {
		if strings.Contains(err.Error(), "No node found") {
			return 0, ErrNoElementAtPoint
		}
		return 0, fmt.Errorf("cdp: resolve node: %w", err)
	}
	var locRes struct {
		BackendNodeID int `json:"backendNodeId"`
	}
	if err := json.Unmarshal(loc, &locRes); err != nil || locRes.BackendNodeID == 0 {
		return 0, ErrNoElementAtPoint
	}
	pushed, err := c.call(ctx, "DOM.pushNodesByBackendIdsToFrontend", map[string]any{
		"backendNodeIds": []int{locRes.BackendNodeID},
	})
	if err != nil {
		return 0, fmt.Errorf("cdp: resolve node: %w", err)
	}
	var pushRes struct {
		NodeIDs []int `json:"nodeIds"`
	}
	if err := json.Unmarshal(pushed, &pushRes); err != nil || len(pushRes.NodeIDs) == 0 || pushRes.NodeIDs[0] == 0 {
		return 0, ErrNoElementAtPoint
	}
	return pushRes.NodeIDs[0], nil
}

const keyNodeID = "nodeId"

func boxFor(ctx context.Context, c *inspectConn, nodeID int) BoundingBox {
	raw, err := c.call(ctx, "DOM.getBoxModel", map[string]any{keyNodeID: nodeID})
	if err != nil {
		return BoundingBox{}
	}
	var r struct {
		Model struct {
			Content []float64 `json:"content"` // quad: x1,y1,x2,y2,x3,y3,x4,y4
		} `json:"model"`
	}
	if json.Unmarshal(raw, &r) != nil || len(r.Model.Content) < 8 {
		return BoundingBox{}
	}
	q := r.Model.Content[:8]
	minX, maxX, minY, maxY := q[0], q[0], q[1], q[1]
	for i := 2; i < 8; i += 2 {
		minX, maxX = min(minX, q[i]), max(maxX, q[i])
		minY, maxY = min(minY, q[i+1]), max(maxY, q[i+1])
	}
	return BoundingBox{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}
}

func outerHTMLFor(ctx context.Context, c *inspectConn, nodeID int) string {
	raw, err := c.call(ctx, "DOM.getOuterHTML", map[string]any{keyNodeID: nodeID})
	if err != nil {
		return ""
	}
	var r struct {
		OuterHTML string `json:"outerHTML"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return ""
	}
	return truncateHTML(r.OuterHTML)
}

// truncateHTML cuts s to MaxInspectHTMLBytes (marker included) on a rune boundary.
func truncateHTML(s string) string {
	if len(s) <= MaxInspectHTMLBytes {
		return s
	}
	cut := MaxInspectHTMLBytes - len(htmlTruncationMark)
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut] + htmlTruncationMark
}

func computedCSSFor(ctx context.Context, c *inspectConn, nodeID int) []CSSProperty {
	raw, err := c.call(ctx, "CSS.getComputedStyleForNode", map[string]any{keyNodeID: nodeID})
	if err != nil {
		return []CSSProperty{}
	}
	var r struct {
		ComputedStyle []CSSProperty `json:"computedStyle"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return []CSSProperty{}
	}
	return selectCSS(r.ComputedStyle)
}

// selectCSS puts priorityCSSProps first (shorthands like "border" are matched
// by prefix when absent, e.g. border-top-width) and fills the rest in order.
func selectCSS(all []CSSProperty) []CSSProperty {
	byName := make(map[string]string, len(all))
	for _, p := range all {
		byName[p.Name] = p.Value
	}
	out := make([]CSSProperty, 0, MaxInspectCSSProps)
	used := map[string]bool{}
	for _, name := range priorityCSSProps {
		if v, ok := byName[name]; ok {
			out = append(out, CSSProperty{name, v})
			used[name] = true
			continue
		}
		// Computed styles only expose longhands; surface them for the shorthand.
		for _, p := range all {
			if strings.HasPrefix(p.Name, name+"-") && !used[p.Name] && len(out) < len(priorityCSSProps)+4 {
				out = append(out, p)
				used[p.Name] = true
			}
		}
	}
	for _, p := range all {
		if len(out) >= MaxInspectCSSProps {
			break
		}
		if !used[p.Name] {
			out = append(out, p)
			used[p.Name] = true
		}
	}
	if len(out) > MaxInspectCSSProps {
		out = out[:MaxInspectCSSProps]
	}
	return out
}

// scrollOffset returns the visual viewport's page offset: box-model quads are
// viewport-relative while captureScreenshot clips are page-relative.
func scrollOffset(ctx context.Context, c *inspectConn) (float64, float64) {
	raw, err := c.call(ctx, "Page.getLayoutMetrics", nil)
	if err != nil {
		return 0, 0
	}
	var r struct {
		Viewport struct {
			PageX float64 `json:"pageX"`
			PageY float64 `json:"pageY"`
		} `json:"cssVisualViewport"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return 0, 0
	}
	return r.Viewport.PageX, r.Viewport.PageY
}

func screenshotFor(ctx context.Context, c *inspectConn, b BoundingBox) string {
	dx, dy := scrollOffset(ctx, c)
	b.X += dx
	b.Y += dy
	raw, err := c.call(ctx, "Page.captureScreenshot", map[string]any{
		"format": "jpeg", "quality": 80,
		"clip": map[string]any{"x": b.X, "y": b.Y, "width": b.Width, "height": b.Height, "scale": 1},
	})
	if err != nil {
		return ""
	}
	var r struct {
		Data string `json:"data"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return ""
	}
	return r.Data
}
