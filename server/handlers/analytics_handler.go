package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/analytics"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/analyticsevent"
)

// validCategories is the set of accepted event_category values.
var validCategories = map[string]bool{
	"user_action": true,
	"performance": true,
	"navigation":  true,
	"rpc":         true,
}

// analyticsEventRequest matches the frontend AnalyticsEvent JSON shape.
type analyticsEventRequest struct {
	ID           string            `json:"id,omitempty"`
	EventName    string            `json:"name"`
	EventCategory string           `json:"category"`
	DurationMs   *int64            `json:"duration_ms,omitempty"`
	SessionID    string            `json:"session_id,omitempty"`
	Page         string            `json:"page,omitempty"`
	Component    string            `json:"component,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
}

// analyticsBatchRequest is the POST /api/analytics request body.
type analyticsBatchRequest struct {
	Events []analyticsEventRequest `json:"events"`
}

// topEventEntry is one entry in the top-events list.
type topEventEntry struct {
	EventName   string  `json:"event_name"`
	Count       int     `json:"count"`
	AvgDuration float64 `json:"avg_duration_ms,omitempty"`
}

// rpcLatencyEntry holds p50/p95/p99 latency for one RPC method.
type rpcLatencyEntry struct {
	EventName string  `json:"event_name"`
	P50       float64 `json:"p50_ms"`
	P95       float64 `json:"p95_ms"`
	P99       float64 `json:"p99_ms"`
	Count     int     `json:"count"`
}

// pageViewEntry holds view count for one page.
type pageViewEntry struct {
	Page  string `json:"page"`
	Count int    `json:"count"`
}

// analyticsSummaryResponse is the GET /api/analytics/summary response body.
// It matches the FR-5 JSON shape defined in the requirements.
type analyticsSummaryResponse struct {
	From       time.Time         `json:"from"`
	To         time.Time         `json:"to"`
	TotalCount int               `json:"total_count"`
	TopEvents  []topEventEntry   `json:"top_events"`
	RPCLatency []rpcLatencyEntry `json:"rpc_latency"`
	PageViews  []pageViewEntry   `json:"page_views"`
}

// AnalyticsHandler handles POST /api/analytics and GET /api/analytics/summary.
type AnalyticsHandler struct {
	provider analytics.AnalyticsProvider
	limiter  *rateLimiter
	client   *ent.Client // may be nil when only LogAnalyticsProvider is available
}

// NewAnalyticsHandler creates a new AnalyticsHandler with a 1000/min rate limiter.
func NewAnalyticsHandler(provider analytics.AnalyticsProvider) *AnalyticsHandler {
	if provider == nil {
		provider = analytics.NewLogAnalyticsProvider()
	}
	return &AnalyticsHandler{
		provider: provider,
		limiter: &rateLimiter{
			resetAt: time.Now().Add(time.Minute),
		},
	}
}

// NewAnalyticsHandlerWithClient creates a new AnalyticsHandler that can also serve
// summary queries from the ent client. Use this in production wiring.
func NewAnalyticsHandlerWithClient(provider analytics.AnalyticsProvider, client *ent.Client) *AnalyticsHandler {
	h := NewAnalyticsHandler(provider)
	h.client = client
	return h
}

// RegisterRoutes registers the analytics routes on mux.
func (h *AnalyticsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/analytics", h.HandlePost)
	mux.HandleFunc("GET /api/analytics/summary", h.HandleSummary)
}

// analyticsRateLimiter allows 1000 requests per minute.
func (h *AnalyticsHandler) allow() bool {
	h.limiter.mu.Lock()
	defer h.limiter.mu.Unlock()
	now := time.Now()
	if now.After(h.limiter.resetAt) {
		h.limiter.count = 0
		h.limiter.resetAt = now.Add(time.Minute)
	}
	if h.limiter.count >= 1000 {
		return false
	}
	h.limiter.count++
	return true
}

// HandlePost handles POST /api/analytics.
func (h *AnalyticsHandler) HandlePost(w http.ResponseWriter, r *http.Request) {
	if !h.allow() {
		http.Error(w, "Too many requests", http.StatusTooManyRequests)
		return
	}

	// Cap body to 512 KB.
	r.Body = http.MaxBytesReader(w, r.Body, 512*1024)

	var req analyticsBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if len(req.Events) == 0 {
		http.Error(w, "events is required and must not be empty", http.StatusBadRequest)
		return
	}
	if len(req.Events) > 100 {
		http.Error(w, "batch too large: maximum 100 events per request", http.StatusBadRequest)
		return
	}

	// Validate each event before recording any of them.
	for i, ev := range req.Events {
		if ev.EventName == "" {
			http.Error(w, "event name is required", http.StatusBadRequest)
			return
		}
		if !validCategories[ev.EventCategory] {
			log.Info("analytics invalid category on event", "category", ev.EventCategory, "index", i)
			http.Error(w, "invalid event category: must be one of user_action, performance, navigation, rpc", http.StatusBadRequest)
			return
		}
	}

	ctx := r.Context()
	for _, ev := range req.Events {
		// Sanitize event name: strip newlines to prevent log injection.
		safeName := strings.ReplaceAll(ev.EventName, "\n", `\n`)
		safeName = strings.ReplaceAll(safeName, "\r", `\r`)

		aerr := h.provider.Record(ctx, analytics.Event{
			ID:            ev.ID,
			EventName:     safeName,
			EventCategory: ev.EventCategory,
			SessionID:     ev.SessionID,
			DurationMs:    ev.DurationMs,
			Page:          ev.Page,
			Component:     ev.Component,
			Labels:        ev.Labels,
		})
		if aerr != nil {
			log.Error("analytics record error (continuing)", "err", aerr)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleSummary handles GET /api/analytics/summary.
func (h *AnalyticsHandler) HandleSummary(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	from := now.AddDate(0, 0, -7)
	to := now

	q := r.URL.Query()
	if raw := q.Get("from"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			from = t
		}
	}
	if raw := q.Get("to"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			to = t
		}
	}

	categoryFilter := q.Get("category")

	// If no ent client, return an empty summary.
	if h.client == nil {
		resp := analyticsSummaryResponse{
			From:       from,
			To:         to,
			TotalCount: 0,
			TopEvents:  []topEventEntry{},
			RPCLatency: []rpcLatencyEntry{},
			PageViews:  []pageViewEntry{},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Error("analytics/summary encode error", "err", err)
		}
		return
	}

	ctx := r.Context()

	// Build base query with time range.
	query := h.client.AnalyticsEvent.Query().
		Where(
			analyticsevent.CreatedAtGTE(from),
			analyticsevent.CreatedAtLTE(to),
		)

	if categoryFilter != "" {
		query = query.Where(analyticsevent.EventCategoryEQ(categoryFilter))
	}

	events, err := query.All(ctx)
	if err != nil {
		log.Error("analytics/summary query failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Aggregate in Go.
	type eventAgg struct {
		count       int
		totalDurMs  int64
		durCount    int
	}

	eventMap := make(map[string]*eventAgg)
	rpcDurations := make(map[string][]int64) // event_name → []duration_ms for rpc category
	pageMap := make(map[string]int)

	for _, ev := range events {
		name := ev.EventName
		if _, ok := eventMap[name]; !ok {
			eventMap[name] = &eventAgg{}
		}
		agg := eventMap[name]
		agg.count++
		if ev.DurationMs != nil {
			agg.totalDurMs += *ev.DurationMs
			agg.durCount++
		}

		if ev.EventCategory == "rpc" && ev.DurationMs != nil {
			rpcDurations[name] = append(rpcDurations[name], *ev.DurationMs)
		}

		if ev.EventCategory == "navigation" && ev.Page != "" {
			pageMap[ev.Page]++
		}
	}

	// Build top events (sorted by count desc, top 10).
	topEvents := make([]topEventEntry, 0, len(eventMap))
	for name, agg := range eventMap {
		entry := topEventEntry{
			EventName: name,
			Count:     agg.count,
		}
		if agg.durCount > 0 {
			entry.AvgDuration = float64(agg.totalDurMs) / float64(agg.durCount)
		}
		topEvents = append(topEvents, entry)
	}
	sort.Slice(topEvents, func(i, j int) bool {
		if topEvents[i].Count != topEvents[j].Count {
			return topEvents[i].Count > topEvents[j].Count
		}
		return topEvents[i].EventName < topEvents[j].EventName
	})
	if len(topEvents) > 10 {
		topEvents = topEvents[:10]
	}

	// Build RPC latency percentiles.
	rpcLatency := make([]rpcLatencyEntry, 0, len(rpcDurations))
	for name, durations := range rpcDurations {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		entry := rpcLatencyEntry{
			EventName: name,
			Count:     len(durations),
			P50:       percentile(durations, 50),
			P95:       percentile(durations, 95),
			P99:       percentile(durations, 99),
		}
		rpcLatency = append(rpcLatency, entry)
	}
	sort.Slice(rpcLatency, func(i, j int) bool {
		return rpcLatency[i].EventName < rpcLatency[j].EventName
	})

	// Build page views (sorted by count desc).
	pageViews := make([]pageViewEntry, 0, len(pageMap))
	for page, count := range pageMap {
		pageViews = append(pageViews, pageViewEntry{Page: page, Count: count})
	}
	sort.Slice(pageViews, func(i, j int) bool {
		if pageViews[i].Count != pageViews[j].Count {
			return pageViews[i].Count > pageViews[j].Count
		}
		return pageViews[i].Page < pageViews[j].Page
	})

	resp := analyticsSummaryResponse{
		From:       from,
		To:         to,
		TotalCount: len(events),
		TopEvents:  topEvents,
		RPCLatency: rpcLatency,
		PageViews:  pageViews,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Error("analytics/summary encode error", "err", err)
	}
}

// percentile returns the p-th percentile of a sorted int64 slice.
// p should be in [0, 100]. Returns 0 for empty slices.
func percentile(sorted []int64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	// Use the nearest-rank method.
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return float64(sorted[idx])
}
