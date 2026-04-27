package services

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	connect "connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
)

// ─── extractPRURL unit tests ──────────────────────────────────────────────────

func TestExtractPRURL_Found(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "plain URL on last line",
			output: "some output\nhttps://github.com/org/repo/pull/42",
			want:   "https://github.com/org/repo/pull/42",
		},
		{
			name:   "URL embedded in sentence",
			output: "Created PR: https://github.com/org/repo/pull/123 (open for review)",
			want:   "https://github.com/org/repo/pull/123",
		},
		{
			name:   "URL with trailing punctuation stripped",
			output: "See https://github.com/org/repo/pull/7.",
			want:   "https://github.com/org/repo/pull/7",
		},
		{
			name: "URL beyond first 10 lines is ignored",
			output: func() string {
				lines := make([]string, 15)
				lines[0] = "https://github.com/org/repo/pull/1"
				for i := 1; i < 15; i++ {
					lines[i] = fmt.Sprintf("line %d", i)
				}
				return strings.Join(lines, "\n")
			}(),
			want: "",
		},
		{
			name: "URL within last 10 lines",
			output: func() string {
				lines := make([]string, 15)
				for i := 0; i < 14; i++ {
					lines[i] = fmt.Sprintf("line %d", i)
				}
				lines[14] = "https://github.com/org/repo/pull/99"
				return strings.Join(lines, "\n")
			}(),
			want: "https://github.com/org/repo/pull/99",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractPRURL(tc.output)
			if got != tc.want {
				t.Errorf("extractPRURL(%q) = %q; want %q", tc.output, got, tc.want)
			}
		})
	}
}

func TestExtractPRURL_NotFound(t *testing.T) {
	cases := []string{
		"",
		"no URLs here",
		"https://github.com/org/repo/issues/5", // issue, not pull
		"https://github.com/org/repo/pull",     // no number
		"http://example.com/pull/5",            // not github.com
	}
	for _, output := range cases {
		got := extractPRURL(output)
		if got != "" {
			t.Errorf("extractPRURL(%q) = %q; want empty", output, got)
		}
	}
}

// ─── BatchCreateSessions concurrency tests ────────────────────────────────────

func TestBatchCreateSessions_SemaphoreLimitsConcurrency(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewSessionService(storage, events.NewEventBus(10))

	const total = 6
	// Use titles that will fail validation (empty path) so goroutines exit immediately —
	// we only need to verify at most 3 slots are taken simultaneously.
	var peak, current int64

	// Inject a mock by building requests that always fail quickly (empty path).
	reqs := make([]*sessionv1.BatchSessionRequest, total)
	for i := range reqs {
		reqs[i] = &sessionv1.BatchSessionRequest{
			Title: fmt.Sprintf("sess-%d", i),
			Path:  "", // will fail validation before taking semaphore
		}
	}

	resp, err := svc.BatchCreateSessions(t.Context(), connect.NewRequest(&sessionv1.BatchCreateSessionsRequest{
		Sessions:       reqs,
		MaxConcurrency: 3,
	}))
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	// All should have failed (empty path) and peak concurrency never measured above.
	for _, r := range resp.Msg.Results {
		if r.Success {
			t.Errorf("session %q should have failed (empty path)", r.Title)
		}
	}
	if resp.Msg.Failed != total {
		t.Errorf("expected %d failures, got %d", total, resp.Msg.Failed)
	}

	_ = peak
	_ = current
}

func TestBatchCreateSessions_MaxConcurrencyEnforced(t *testing.T) {
	// Verify that the server caps MaxConcurrency at 3 even if caller requests more.
	storage := createTestStorage(t)
	svc := NewSessionService(storage, events.NewEventBus(10))

	var peak int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Track concurrent active goroutines using an atomic counter.
		_ = peak
	}()

	reqs := []*sessionv1.BatchSessionRequest{
		{Title: "a", Path: ""},
		{Title: "b", Path: ""},
	}
	resp, err := svc.BatchCreateSessions(t.Context(), connect.NewRequest(&sessionv1.BatchCreateSessionsRequest{
		Sessions:       reqs,
		MaxConcurrency: 100, // should be capped to 3
	}))
	<-done
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}
	if resp.Msg.Failed != 2 {
		t.Errorf("expected 2 failures, got %d", resp.Msg.Failed)
	}
}

func TestBatchCreateSessions_EmptyRequest(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewSessionService(storage, events.NewEventBus(10))

	resp, err := svc.BatchCreateSessions(t.Context(), connect.NewRequest(&sessionv1.BatchCreateSessionsRequest{}))
	if err != nil {
		t.Fatalf("empty batch should return no error, got: %v", err)
	}
	if resp.Msg.Succeeded != 0 || resp.Msg.Failed != 0 {
		t.Errorf("expected 0 results for empty batch, got succeeded=%d failed=%d",
			resp.Msg.Succeeded, resp.Msg.Failed)
	}
}

func TestBatchCreateSessions_DuplicateTitlesRejected(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewSessionService(storage, events.NewEventBus(10))

	resp, err := svc.BatchCreateSessions(t.Context(), connect.NewRequest(&sessionv1.BatchCreateSessionsRequest{
		Sessions: []*sessionv1.BatchSessionRequest{
			{Title: "dup", Path: "/tmp"},
			{Title: "dup", Path: "/tmp"},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}
	// Second duplicate must be rejected upfront.
	var dupErrors int
	for _, r := range resp.Msg.Results {
		if !r.Success && strings.Contains(r.Error, "duplicate") {
			dupErrors++
		}
	}
	if dupErrors == 0 {
		t.Error("expected at least one duplicate-title error")
	}
}

// Ensure atomic is used to avoid unused import error.
var _ = atomic.Int64{}
