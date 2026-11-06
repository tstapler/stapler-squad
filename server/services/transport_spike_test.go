package services

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/middleware"
)

// S-T1 (ADR-036): what a range-sized unary response costs on the wire through the
// production middleware layers (otelhttp, Logging, CORS, Compress) and the
// production Connect handler options. Throwaway handler: test files only.

type bigPayloadStub struct {
	sessionv1connect.UnimplementedSessionServiceHandler
	content string
}

func (s bigPayloadStub) GetFileContent(context.Context, *connect.Request[sessionv1.GetFileContentRequest]) (*connect.Response[sessionv1.GetFileContentResponse], error) {
	return connect.NewResponse(&sessionv1.GetFileContentResponse{Content: s.content, Encoding: "utf-8"}), nil
}

// goSourceText concatenates distinct non-test Go files so the sample is real,
// non-repeating code (a repeated file would overstate the compression ratio).
func goSourceText(t *testing.T, size int) string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	sort.Strings(files)
	var b strings.Builder
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		b.Write(src)
		if b.Len() >= size {
			return b.String()[:size]
		}
	}
	t.Fatalf("only %d bytes of distinct source", b.Len())
	return ""
}

func wordsText(size int) string {
	return string(bytes.Repeat(fixtureText(rand.New(rand.NewSource(fixtureSeed)), 50), size/100+1))[:size]
}

type wireSample struct {
	encoding string
	bytes    int
	median   time.Duration
}

// wireCost POSTs a Connect unary JSON call with the given extra headers and
// returns the on-the-wire body size (HTTP transport decompression disabled).
func wireCost(t *testing.T, url string, headers map[string]string) wireSample {
	t.Helper()
	hc := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	var durs []time.Duration
	var out wireSample
	for i := 0; i < 15; i++ {
		req, err := http.NewRequest(http.MethodPost, url+"/api/session.v1.SessionService/GetFileContent", strings.NewReader(`{"sessionId":"s","path":"a"}`))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Connect-Protocol-Version", "1")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		start := time.Now()
		resp, err := hc.Do(req)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		durs = append(durs, time.Since(start))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		out.encoding, out.bytes = resp.Header.Get("Content-Encoding"), len(body)
		if out.encoding == "gzip" {
			zr, err := gzip.NewReader(bytes.NewReader(body))
			require.NoError(t, err)
			_, err = io.Copy(io.Discard, zr)
			require.NoError(t, err)
		}
	}
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	out.median = durs[len(durs)/2]
	return out
}

func newWireServer(t *testing.T, content string) string {
	t.Helper()
	path, h := sessionv1connect.NewSessionServiceHandler(bigPayloadStub{content: content}, connectOptionsLikeServer()...)
	mux := http.NewServeMux()
	mux.Handle("/api"+path, http.StripPrefix("/api", h))
	chain := otelhttp.NewHandler(middleware.Logging(middleware.CORSWithOrigins(nil)(middleware.Compress(mux))), "spike")
	ts := httptest.NewServer(chain)
	t.Cleanup(ts.Close)
	return ts.URL
}

// connectOptionsLikeServer: server.ConnectOptions only adds interceptors (error
// recorder, otelconnect) and sets no compression or size option, so the defaults
// under test are connect-go's own.
func connectOptionsLikeServer() []connect.HandlerOption { return nil }

func TestTransportSpike_WireCost_ShouldRecordContentEncodingAndSize_For128KiBAnd1MiB(t *testing.T) {
	browser := "gzip, deflate, br, zstd" // what a browser sends; connect-web sets nothing else
	for _, size := range []int{128 << 10, 1 << 20} {
		for name, text := range map[string]string{"gosource": goSourceText(t, size), "words": wordsText(size)} {
			url := newWireServer(t, text)
			identity := wireCost(t, url, nil)
			br := wireCost(t, url, map[string]string{"Accept-Encoding": browser})
			t.Logf("size=%d text=%s no-Accept-Encoding: encoding=%q wire=%d median=%v | browser Accept-Encoding: encoding=%q wire=%d median=%v ratio=%.3f",
				size, name, identity.encoding, identity.bytes, identity.median, br.encoding, br.bytes, br.median, float64(br.bytes)/float64(identity.bytes))
			require.Greater(t, identity.bytes, size, "identity body carries the full text plus JSON framing")
		}
	}
}

func TestTransportSpike_WebSocket_ShouldReportPermessageDeflateNegotiation(t *testing.T) {
	negotiate := func(t *testing.T, up websocket.Upgrader) string {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := up.Upgrade(w, r, nil)
			if err == nil {
				c.Close()
			}
		}))
		defer srv.Close()
		d := websocket.Dialer{EnableCompression: true}
		c, resp, err := d.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
		require.NoError(t, err)
		defer c.Close()
		return resp.Header.Get("Sec-Websocket-Extensions")
	}
	prod := negotiate(t, wsUpgrader)
	enabled := negotiate(t, websocket.Upgrader{EnableCompression: true})
	t.Logf("production wsUpgrader: Sec-WebSocket-Extensions=%q; with EnableCompression: %q", prod, enabled)
	require.Empty(t, prod, "production upgrader does not enable permessage-deflate")
	require.Contains(t, enabled, "permessage-deflate")
}

// The S-T2 prototypes must be right before their speed means anything.
func TestTransportSpike_SpliceAndIndex_ShouldMatchInMemoryOracle(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	old := bytes.Repeat([]byte("line of text\n"), 5000)
	require.NoError(t, os.WriteFile(src, old, 0o644))

	sum, lines, index, err := indexAndHash(bytes.NewReader(old))
	require.NoError(t, err)
	require.Equal(t, sha256.Sum256(old), sum)
	require.Equal(t, 5000, lines)
	require.Len(t, index, 5000/indexEveryLine)
	require.EqualValues(t, 13*indexEveryLine, index[0])

	insert := []byte("INSERTED")
	off, del := int64(1000), int64(20)
	oldSum, newSum, err := splice(src, dir, off, del, insert)
	require.NoError(t, err)
	want := append(append(append([]byte{}, old[:off]...), insert...), old[off+del:]...)
	require.Equal(t, sha256.Sum256(old), oldSum)
	require.Equal(t, sha256.Sum256(want), newSum)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "temp file is removed after the splice")
}
