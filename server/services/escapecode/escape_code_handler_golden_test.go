package escapecode

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/analytics"
)

// Recorded from the pre-move handler in server/services (origin/main) with
// ESCAPECODE_GOLDEN_RECORD=1; this file differs there only by the package clause. A diff in
// status, headers, body, captured log records or stored entries is a behaviour change.
const escapeGoldenPath = "testdata/escape_code_handler.golden"

var escapeTimeRe = regexp.MustCompile(`"(firstSeen|lastSeen|exportedAt)":\s*"[^"]*"`)

func escapeGoldenDo(b *strings.Builder, mux *http.ServeMux, method, path, body string) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	hdr := make([]string, 0, 2)
	for _, k := range []string{"Content-Type", "Content-Disposition", "Allow"} {
		if v := rec.Header().Get(k); v != "" {
			hdr = append(hdr, k+"="+v)
		}
	}
	out := escapeTimeRe.ReplaceAllString(rec.Body.String(), `"$1":"T"`)
	b.WriteString(method + " " + path + " " + body + " -> ")
	b.WriteString(strings.Join(append([]string{http.StatusText(rec.Code)}, hdr...), ";"))
	b.WriteString(" body=")
	b.WriteString(strings.ReplaceAll(out, "\n", "\\n"))
	b.WriteString("\n")
}

func TestEscapeCodeHandler_Golden(t *testing.T) {
	var logs bytes.Buffer
	h := slog.NewTextHandler(&logs, &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})
	prev := log.SetSlogDefaultForTest(slog.New(h))
	t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })

	store := analytics.NewEscapeCodeStore()
	handler := &EscapeCodeHandler{store: store}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	var b strings.Builder
	do := func(method, path, body string) { escapeGoldenDo(&b, mux, method, path, body) }

	// disabled store drops records
	store.Record("s1", []byte("\x1b[0m"), analytics.CategorySGR, "reset")
	do("GET", "/api/debug/escape-codes", "")
	do("GET", "/api/debug/escape-codes/status", "")
	do("POST", "/api/debug/escape-codes/toggle", `{"enabled":true}`)
	do("POST", "/api/debug/escape-codes/toggle", `{bad`)
	do("GET", "/api/debug/escape-codes/toggle", "")
	do("GET", "/api/debug/escape-codes/status", "")

	store.Record("s1", []byte("\x1b[0m"), analytics.CategorySGR, "reset")
	store.Record("s2", []byte("\x1b[0m"), analytics.CategorySGR, "reset")
	store.Record("s2", []byte("\x1b[0m"), analytics.CategorySGR, "reset")
	store.Record("s2", []byte("\x1b[2J"), analytics.CategoryErase, "clear")
	for _, p := range []string{
		"/api/debug/escape-codes",
		"/api/debug/escape-codes/",
		"/api/debug/escape-codes/stats",
		"/api/debug/escape-codes/session/s1",
		"/api/debug/escape-codes/session/s1/",
		"/api/debug/escape-codes/session/nope",
		"/api/debug/escape-codes/session/",
		"/api/debug/escape-codes/category/Erase",
		"/api/debug/escape-codes/category/Erase/",
		"/api/debug/escape-codes/category/",
		"/api/debug/escape-codes/category/Bogus",
		"/api/debug/escape-codes/export",
		"/api/debug/escape-codes/unknown",
		"/api/debug/escape-codes/status",
	} {
		do("GET", p, "")
	}
	do("PUT", "/api/debug/escape-codes", "")
	do("POST", "/api/debug/escape-codes/stats", "")
	do("POST", "/api/debug/escape-codes/export", "")
	do("POST", "/api/debug/escape-codes/status", "")
	do("POST", "/api/debug/escape-codes/session/s1", "")
	do("POST", "/api/debug/escape-codes/category/CSI", "")
	do("DELETE", "/api/debug/escape-codes", "")
	do("DELETE", "/api/debug/escape-codes/", "")
	do("GET", "/api/debug/escape-codes", "")
	do("GET", "/api/debug/escape-codes/stats", "")
	do("POST", "/api/debug/escape-codes/toggle", `{"enabled":false}`)
	do("GET", "/api/debug/escape-codes/status", "")
	b.WriteString("logs:\n" + logs.String())

	got := b.String()
	if os.Getenv("ESCAPECODE_GOLDEN_RECORD") == "1" {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(escapeGoldenPath, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(escapeGoldenPath)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}

func TestEscapeCodeHandler_NewUsesGlobalStore(t *testing.T) {
	t.Parallel()
	require.Same(t, analytics.GetGlobalStore(), NewEscapeCodeHandler().store)
}
