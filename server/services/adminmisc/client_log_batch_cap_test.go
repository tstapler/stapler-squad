package adminmisc

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tstapler/stapler-squad/log"
)

type countingHandler struct{ n *atomic.Int64 }

func (countingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h countingHandler) Handle(context.Context, slog.Record) error {
	h.n.Add(1)
	return nil
}
func (h countingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h countingHandler) WithGroup(string) slog.Handler      { return h }

// TestClientLogHandler_BatchCapIsExactlyMaxClientLogBatch pins the cap: entries past it are dropped, not logged.
func TestClientLogHandler_BatchCapIsExactlyMaxClientLogBatch(t *testing.T) {
	var logged atomic.Int64
	prev := log.SetSlogDefaultForTest(slog.New(countingHandler{n: &logged}))
	t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })

	for _, tc := range []struct{ sent, want int }{
		{maxClientLogBatch - 1, maxClientLogBatch - 1},
		{maxClientLogBatch, maxClientLogBatch},
		{maxClientLogBatch + 1, maxClientLogBatch},
	} {
		logged.Store(0)
		body := `{"windowId":"w","entries":[` + strings.TrimSuffix(strings.Repeat(`{"level":"warn","message":"x"},`, tc.sent), ",") + `]}`
		if rec := post(NewClientLogHandler(), body); rec.Code != http.StatusNoContent {
			t.Fatalf("sent %d: want 204, got %d", tc.sent, rec.Code)
		}
		if got := int(logged.Load()); got != tc.want {
			t.Errorf("sent %d entries: logged %d, want %d", tc.sent, got, tc.want)
		}
	}
}
