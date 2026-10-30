package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func postEvents(h *AnalyticsHandler, events []map[string]any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(map[string]any{"events": events})
	req := httptest.NewRequest(http.MethodPost, "/api/analytics", bytes.NewReader(b))
	req.RemoteAddr = "127.0.0.1:50000" // every local browser and same-box proxy looks like this
	w := httptest.NewRecorder()
	h.HandlePost(w, req)
	return w
}

func evs(name string, n int) []map[string]any {
	out := make([]map[string]any, n)
	for i := range out {
		out[i] = map[string]any{"name": name, "category": "performance", "labels": map[string]string{"size": "2"}}
	}
	return out
}

// TestAnalytics_TabsOnOneLoopbackAddressAreNotThrottled pins the single-address ceiling: ten tabs
// on one box (all 127.0.0.1) flushing every 20s for a minute (3 requests each) at 100 events a
// request send 3,000 events and must all be accepted.
func TestAnalytics_TabsOnOneLoopbackAddressAreNotThrottled(t *testing.T) {
	h := newAnalyticsHandlerForTest()
	for i := 0; i < 10*3; i++ {
		require.Equal(t, http.StatusNoContent, postEvents(h, evs("page_view", 100)).Code, "request %d", i+1)
	}
}

func TestAnalytics_BusyBoxCeilingIsTheDocumentedBudget(t *testing.T) {
	h := newAnalyticsHandlerForTest()
	var accepted int
	for i := 0; i < 1000; i++ { // as many 100-event requests as the request bucket allows
		if postEvents(h, evs("page_view", 100)).Code == http.StatusNoContent {
			accepted++
		}
	}
	assert.Equal(t, analyticsEventsPerMinute/100, accepted, "the event budget, not the request bucket, is the ceiling for full batches")
}

func TestAnalytics_GlobalMetricEventBudgetCapsCmEvents(t *testing.T) {
	h := newAnalyticsHandlerForTest()
	h.cmEvents.resetAt = time.Now().Add(time.Minute)
	h.cmEvents.count = metricEventsPerMinute - 5
	assert.Equal(t, http.StatusNoContent, postEvents(h, evs("cm.batch", 5)).Code)
	assert.Equal(t, http.StatusTooManyRequests, postEvents(h, evs("cm.batch", 1)).Code)
	assert.Equal(t, http.StatusNoContent, postEvents(h, evs("page_view", 1)).Code, "non-cm events are not charged to the cm budget")
}

func TestAnalytics_RejectedRequestsChargeNoEventBudget(t *testing.T) {
	h := newAnalyticsHandlerForTest()
	bad := append(evs("page_view", 50), map[string]any{"category": "performance"}) // missing name: 400
	require.Equal(t, http.StatusBadRequest, postEvents(h, bad).Code)
	assert.Equal(t, 0, h.events.count, "a batch that fails validation consumes no event budget")

	h.events.resetAt = time.Now().Add(time.Minute)
	h.events.count = analyticsEventsPerMinute
	require.Equal(t, http.StatusTooManyRequests, postEvents(h, evs("cm.batch", 1)).Code)
	assert.Equal(t, 0, h.cmEvents.count, "a batch refused by the total event budget does not charge the cm budget")
}

func TestAnalytics_BodyCapAppliesToCmEvents(t *testing.T) {
	h := newAnalyticsHandlerForTest()
	big := strings.Repeat("a", 600*1024)
	body := `{"events":[{"name":"cm.ws","category":"performance","labels":{"kind":"` + big + `"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/analytics", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.HandlePost(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMinuteBudget_RefusedTakeChargesNothingAndWindowResets(t *testing.T) {
	b := &minuteBudget{limit: 10}
	assert.True(t, b.take(6))
	assert.False(t, b.take(5))
	assert.True(t, b.take(4), "the refused take left the 4 remaining units intact")
	b.resetAt = time.Now().Add(-time.Second)
	assert.True(t, b.take(10), "a new window starts empty")
}
