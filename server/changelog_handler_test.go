package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/pkg/buildinfo"
)

func TestIsSourceBuild(t *testing.T) {
	origV, origB, origC := buildinfo.Version, buildinfo.Branch, buildinfo.Commit
	t.Cleanup(func() { buildinfo.Version, buildinfo.Branch, buildinfo.Commit = origV, origB, origC })

	cases := []struct {
		name, version, branch, commit string
		want                          bool
	}{
		{"release", "2.5.0", "", "", false},
		{"plain go build", "dev", "", "", true},
		{"derived dev version", "dev-abc123", "", "", true},
		{"makefile build", "2.5.0", "main", "abc1234", true},
	}
	for _, c := range cases {
		buildinfo.Version, buildinfo.Branch, buildinfo.Commit = c.version, c.branch, c.commit
		if got := isSourceBuild(); got != c.want {
			t.Errorf("%s: isSourceBuild() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestChangelogRoutes_FlagGatesAPI(t *testing.T) {
	t.Cleanup(func() {
		changelogFlagCache.mu.Lock()
		changelogFlagCache.at = time.Time{}
		changelogFlagCache.mu.Unlock()
	})
	setFlag := func(on bool) {
		changelogFlagCache.mu.Lock()
		changelogFlagCache.at = time.Now()
		changelogFlagCache.vals = changelogFlags{changelog: on}
		changelogFlagCache.mu.Unlock()
	}

	s := &Server{mux: http.NewServeMux()}
	s.registerChangelogHandlers()

	for _, path := range []string{"/api/changelog", "/api/update-check"} {
		setFlag(false)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s with flag off: status %d, want 404", path, rec.Code)
		}

		setFlag(true)
		rec = httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s with flag on: status %d, want 200", path, rec.Code)
		}
	}
}
