package webhooks

import (
	"log/slog"
	"os"
	"testing"

	ssqlog "github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// TestMain gives this binary the hermetic setup server/services' own tests run under: no non-loopback network, an
// isolated Claude CLI and state dir, and a fixed-answer DNS resolver for the SSRF checks.
func TestMain(m *testing.M) {
	os.Exit(svctest.Main(m, func() { lookupIPAddr = svctest.HermeticLookupIPAddr }))
}

// captureInfoLog redirects the log package's slog seam to a buffer; the returned func restores it and returns the output.
func captureInfoLog() func() string {
	svctest.SlogDefaultMu.Lock()
	buf := &svctest.SyncLogBuffer{}
	h := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	original := ssqlog.SetSlogDefaultForTest(slog.New(h))
	return func() string {
		ssqlog.SetSlogDefaultForTest(original)
		svctest.SlogDefaultMu.Unlock()
		return buf.String()
	}
}
