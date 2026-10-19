package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStaticTestFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":                  {Data: []byte("<root>")},
		"rules/index.html":            {Data: []byte("<rules>")},
		"_next/static/chunks/a-1.js":  {Data: []byte("console.log(1)")},
		"_next/static/chunks/b-2.css": {Data: []byte("body{}")},
		"favicon.ico":                 {Data: []byte("ico")},
	}
}

func getStatic(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	StaticFileServer(newStaticTestFS(), "index.html").ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

func TestStaticFileServer_should_ServeRouteDirectoryIndexWithoutRedirect(t *testing.T) {
	w := getStatic(t, "/rules")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "<rules>", w.Body.String())
	assert.Equal(t, "no-cache, no-store, must-revalidate", w.Header().Get("Cache-Control"))
}

func TestStaticFileServer_should_ServeRootIndex_When_PathIsUnknown(t *testing.T) {
	w := getStatic(t, "/nonexistent")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "<root>", w.Body.String())
	assert.Equal(t, "no-cache, no-store, must-revalidate", w.Header().Get("Cache-Control"))
}

func TestStaticFileServer_should_ServeHashedAssetAsImmutable(t *testing.T) {
	w := getStatic(t, "/_next/static/chunks/a-1.js")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "console.log(1)", w.Body.String())
	assert.Equal(t, "public, max-age=31536000, immutable", w.Header().Get("Cache-Control"))
}

func TestStaticFileServer_should_NotCacheAssetOutsideNextPrefix(t *testing.T) {
	w := getStatic(t, "/favicon.ico")
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Header().Get("Cache-Control"), "immutable")
}

func TestStaticFileServer_should_Return404_When_AssetMissingUnderNextPrefix(t *testing.T) {
	w := getStatic(t, "/_next/static/chunks/old-9.js")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.NotContains(t, w.Body.String(), "<root>", "a stale chunk must not be answered with HTML")
}

func TestStaticFileServer_should_KeepSPAFallback_When_NonAssetPathMissing(t *testing.T) {
	w := getStatic(t, "/backlog/board")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "<root>", w.Body.String())
}

func TestStaticFileServer_should_NotCacheIndexHTMLFile(t *testing.T) {
	// A direct hit on a non-root html file takes the file branch and must carry no-cache.
	fsys := newStaticTestFS()
	fsys["about.html"] = &fstest.MapFile{Data: []byte("<about>")}
	w := httptest.NewRecorder()
	StaticFileServer(fsys, "index.html").ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/about.html", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "no-cache, no-store, must-revalidate", w.Header().Get("Cache-Control"))
}
