package backlogsvc

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

func TestConfigSurfaceProof_StuckAllowAutoMerge_ChangesListedField(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "fake-token")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"allow_auto_merge":%t,"allow_squash_merge":true}`, strings.Contains(r.URL.Path, "/prf-allowed"))
	}))
	t.Cleanup(ts.Close)
	t.Cleanup(svctest.ResetGhBaseURL(ts))

	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		prURL := fmt.Sprintf("https://github.com/prf-owner/%s/pull/1", setting.(string))
		items := stuckAutoMergeSvc(t, githubpkg.RepoSettings, prURL)
		require.Len(t, items, 1)
		require.NotNil(t, items[0].AllowAutoMerge)
		return items[0].GetAllowAutoMerge()
	}, "prf-denied", "prf-allowed")
}
