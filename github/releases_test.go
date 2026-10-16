package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReleaseFetcher_ConditionalRequestsReuseETag(t *testing.T) {
	var ifNoneMatch []string
	var paths []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		ifNoneMatch = append(ifNoneMatch, r.Header.Get("If-None-Match"))
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(`{"tag_name":"v2.6.0","html_url":"https://example.com/rel","published_at":"2026-10-15T00:00:00Z"}`))
	}))
	defer ts.Close()
	defer SetGhBaseURLForTest(ts.URL)()

	f := NewReleaseFetcher("acme/widgets")

	rel, notModified, err := f.Latest(context.Background())
	require.NoError(t, err)
	assert.False(t, notModified)
	assert.Equal(t, "2.6.0", rel.Version)
	assert.Equal(t, "v2.6.0", rel.TagName)
	assert.Equal(t, "https://example.com/rel", rel.HTMLURL)

	rel2, notModified, err := f.Latest(context.Background())
	require.NoError(t, err)
	assert.True(t, notModified, "second call must be a 304")
	assert.Equal(t, rel, rel2)

	assert.Equal(t, []string{"", `"v1"`}, ifNoneMatch)
	assert.Equal(t, []string{"/repos/acme/widgets/releases/latest", "/repos/acme/widgets/releases/latest"}, paths)
}

func TestReleaseFetcher_ErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	defer SetGhBaseURLForTest(ts.URL)()

	_, _, err := NewReleaseFetcher("acme/widgets").Latest(context.Background())
	require.Error(t, err)
}
