package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func postFrom(h *AnalyticsHandler, remote string, events []map[string]any, headers map[string]string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(map[string]any{"events": events})
	req := httptest.NewRequest(http.MethodPost, "/api/analytics", bytes.NewReader(b))
	req.RemoteAddr = remote
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.HandlePost(w, req)
	return w
}

func cmEvents(n int) []map[string]any {
	evs := make([]map[string]any, n)
	for i := range evs {
		evs[i] = map[string]any{"name": "cm.batch", "category": "performance", "labels": map[string]string{"size": "2"}}
	}
	return evs
}

func TestAnalytics_NoisyClientIsLimitedWithoutBlankingOthers(t *testing.T) {
	h := newAnalyticsHandlerForTest()
	var limited int
	for i := 0; i < clientRequestsPerMinute+50; i++ {
		if postFrom(h, "198.51.100.7:4000", cmEvents(1), nil).Code == http.StatusTooManyRequests {
			limited++
		}
	}
	assert.Equal(t, 50, limited, "the flooder is refused past its own budget")
	assert.Equal(t, http.StatusNoContent, postFrom(h, "203.0.113.9:5000", cmEvents(1), nil).Code,
		"a second client still gets through while the first is limited")
}

func TestAnalytics_ForwardedForCannotEscapeTheClientLimit(t *testing.T) {
	h := newAnalyticsHandlerForTest()
	var limited int
	for i := 0; i < clientRequestsPerMinute+10; i++ {
		spoof := map[string]string{"X-Forwarded-For": fmt.Sprintf("10.0.%d.%d", i/250, i%250)}
		if postFrom(h, "198.51.100.8:4000", cmEvents(1), spoof).Code == http.StatusTooManyRequests {
			limited++
		}
	}
	assert.Equal(t, 10, limited, "rotating X-Forwarded-For must not mint fresh buckets")
}

func TestAnalytics_PerClientEventBudgetCountsEventsNotRequests(t *testing.T) {
	h := newAnalyticsHandlerForTest()
	var accepted int
	for i := 0; i < 100; i++ { // 100 requests x 100 events = 10k events, far under 300 requests
		if postFrom(h, "198.51.100.9:4000", cmEvents(100), nil).Code == http.StatusNoContent {
			accepted++
		}
	}
	assert.Equal(t, clientEventsPerMinute/100, accepted)
}

func TestAnalytics_GlobalMetricEventBudgetCapsCmEvents(t *testing.T) {
	h := newAnalyticsHandlerForTest()
	h.cmResets = time.Now().Add(time.Minute)
	h.cmCount = globalMetricEventsPerMinute - 5
	assert.Equal(t, http.StatusNoContent, postFrom(h, "198.51.100.10:1", cmEvents(5), nil).Code)
	assert.Equal(t, http.StatusTooManyRequests, postFrom(h, "198.51.100.11:1", cmEvents(1), nil).Code,
		"cm.* events beyond the shared budget are refused for every client")
	other := []map[string]any{{"name": "page_view", "category": "navigation"}}
	assert.Equal(t, http.StatusNoContent, postFrom(h, "198.51.100.12:1", other, nil).Code, "non-cm events are not charged to the cm budget")
}

func TestAnalytics_ClientMapIsBounded(t *testing.T) {
	l := newPerClientLimiter()
	for i := 0; i < maxTrackedClients*2; i++ {
		l.allowRequest(fmt.Sprintf("10.1.%d.%d", i/250, i%250))
	}
	assert.LessOrEqual(t, len(l.clients), maxTrackedClients+1, "tracked clients are capped, extras share the overflow bucket")

	now := time.Now()
	l.now = func() time.Time { return now.Add(2 * time.Minute) }
	l.allowRequest("fresh")
	assert.Less(t, len(l.clients), maxTrackedClients, "expired buckets are purged when the map is full")
}

func TestAnalytics_BodyCapAppliesToCmEvents(t *testing.T) {
	h := newAnalyticsHandlerForTest()
	big := strings.Repeat("a", 600*1024)
	body := `{"events":[{"name":"cm.ws","category":"performance","labels":{"kind":"` + big + `"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/analytics", strings.NewReader(body))
	req.RemoteAddr = "198.51.100.13:1"
	w := httptest.NewRecorder()
	h.HandlePost(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
