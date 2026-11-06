package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ADR-030 relies on this: ConnectRPC frames its own messages, so /api/ responses must reach
// the client uncompressed, with the handler holding the raw ResponseWriter.
func TestCompress_ShouldPassApiPathsThroughUnwrapped_When_AcceptEncodingGzip(t *testing.T) {
	var seen http.ResponseWriter
	h := Compress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		seen = w
		_, _ = w.Write([]byte("plain api body"))
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/session.v1.SessionService/StatFile", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	h.ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.Same(t, rec, seen, "the handler sees the raw writer, not a compressing wrapper")
	assert.Equal(t, "plain api body", rec.Body.String())
}

func TestCompress_ShouldCompressNonApiPaths_When_AcceptEncodingGzip(t *testing.T) {
	h := Compress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("static asset body"))
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	h.ServeHTTP(rec, req)

	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
	zr, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	body, err := io.ReadAll(zr)
	require.NoError(t, err)
	assert.Equal(t, "static asset body", string(body))
}
