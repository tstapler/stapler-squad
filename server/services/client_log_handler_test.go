package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func post(h *ClientLogHandler, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.HandlePost(rec, httptest.NewRequest(http.MethodPost, "/api/client-logs", strings.NewReader(body)))
	return rec
}

func TestClientLogHandler_AcceptsBatch(t *testing.T) {
	h := NewClientLogHandler()
	rec := post(h, `{"windowId":"w1","entries":[{"level":"warn","message":"a"},{"level":"error","message":"b"},{"level":"debug","message":"ignored"}]}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
	}
}

func TestClientLogHandler_RejectsBadJSON(t *testing.T) {
	if rec := post(NewClientLogHandler(), `{`); rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestClientLogHandler_RateLimitsPerMinuteAndResets(t *testing.T) {
	h := NewClientLogHandler()
	clock := time.Unix(1_000_000, 0)
	h.now = func() time.Time { return clock }
	batch := `{"windowId":"w","entries":[` + strings.Repeat(`{"level":"warn","message":"x"},`, 99) + `{"level":"warn","message":"x"}]}`
	for i := 0; i < 6; i++ {
		if rec := post(h, batch); rec.Code != http.StatusNoContent {
			t.Fatalf("batch %d: want 204, got %d", i, rec.Code)
		}
	}
	if rec := post(h, batch); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 over budget, got %d", rec.Code)
	}
	clock = clock.Add(clientLogBudgetInterval)
	if rec := post(h, batch); rec.Code != http.StatusNoContent {
		t.Fatalf("budget should reset after the interval, got %d", rec.Code)
	}
}

func TestClientLogHandler_DroppedLevelsDoNotSpendBudget(t *testing.T) {
	h := NewClientLogHandler()
	h.now = func() time.Time { return time.Unix(1_000_000, 0) }
	junk := `{"windowId":"w","entries":[` + strings.Repeat(`{"level":"debug","message":"x"},`, 99) + `{"level":"debug","message":"x"}]}`
	for i := 0; i < 20; i++ {
		if rec := post(h, junk); rec.Code != http.StatusNoContent {
			t.Fatalf("debug-only batch %d should never be rate limited, got %d", i, rec.Code)
		}
	}
}

func TestStripQuery(t *testing.T) {
	for in, want := range map[string]string{"/a?token=x": "/a", "/a#frag": "/a", "/a": "/a", "": ""} {
		if got := stripQuery(in); got != want {
			t.Errorf("stripQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTruncate_KeepsValidUTF8(t *testing.T) {
	got := truncate("aé", 2) // é is 2 bytes starting at index 1; cutting at 2 would split it
	if !utf8.ValidString(got) || !strings.HasPrefix(got, "a") {
		t.Fatalf("truncate produced invalid UTF-8: %q", got)
	}
}
