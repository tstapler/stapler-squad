package session

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Where a file that names the legacy scalar PR fields reads them from.
const (
	// readsSnapshot: reads Instance.Snapshot().GitHub (the active-PR projection).
	readsSnapshot = "snapshot"
	// readsInstanceData: reads session.InstanceData rows, which carry the stored scalars only.
	readsInstanceData = "instance-data"
)

// legacyPRReaders lists every non-test Go file that names the legacy single-PR
// scalars (GitHubPRNumber, GitHubPRURL, and the SetGitHub* setters). With several
// PRs per session those scalars show only the active PR, so a reader that wants
// "all PRs" or "is every PR done" must use the PR set instead. Add a file here only
// after reading ActivePR; for readsInstanceData entries, after reading the link rows.
var legacyPRReaders = map[string]string{
	"server/adapters/instance_adapter.go":        readsSnapshot,
	"server/services/active_pr_status.go":        readsSnapshot,     // reads ActivePR first; scalars only when the session has no links
	"session/pr_cleanup_gate.go":                 readsInstanceData, // OwnPRsBlockReason (shared with the retention sweeper) reads the link rows; the scalar only decides the no-link and unrepresented cases
	"session/instance_pr_edit.go":                readsSnapshot,     // persists the mirror projected from the set
	"session/pr_legacy_safe.go":                  readsInstanceData, // the shared rule for a legacy number with no link
	"session/pr_set_rules.go":                    readsSnapshot,     // ExistingPRForCreate reads the set; scalars only for a link-less session
	"server/pr_annotations.go":                   readsSnapshot,
	"server/prwatch_sessions.go":                 readsSnapshot,
	"server/reviewqueue/review_queue_manager.go": readsSnapshot,
	"server/services/approval_service.go":        readsSnapshot,
	"server/services/debug_snapshot.go":          readsInstanceData,
	"server/services/session_service_create.go":  readsSnapshot, // writes InstanceOptions only; the instance attaches it
	"server/services/unfinished_work_service.go": readsInstanceData,
	"session/ent_repository.go":                  readsInstanceData,
	"session/instance.go":                        readsSnapshot,
	"session/instance_actor_setters.go":          readsSnapshot,
	"session/instance_pr_set.go":                 readsSnapshot,
	"session/instance_serialization.go":          readsSnapshot,
	"session/instance_snapshot.go":               readsSnapshot,
	"session/instance_terminal.go":               readsSnapshot,
	"session/prpoller/pr_status_poller.go":       readsSnapshot,
	"session/storage.go":                         readsInstanceData,
}

// Whole identifiers only, so names like runGitHubPRURLBackfill do not count.
var legacyPRFieldRe = regexp.MustCompile(`\b(Set)?GitHubPR(Number|URL)\b`)

// skippedDirs hold generated code, other languages or non-source trees.
var skippedLegacyScanDirs = map[string]bool{
	".git": true, ".claude": true, "gen": true, "node_modules": true, "web-app": true,
	"tests": true, "docs": true, "project_plans": true, "vendor": true,
}

func filesNamingLegacyPRFields(t *testing.T, root string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if skippedLegacyScanDirs[d.Name()] || rel == filepath.Join("session", "ent") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if legacyPRFieldRe.Match(src) {
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	})
	require.NoError(t, err)
	sort.Strings(found)
	return found
}

func TestLegacyPRReaders_ShouldMatchAllowList_WhenGrepping27GoFiles(t *testing.T) {
	t.Parallel()
	found := filesNamingLegacyPRFields(t, moduleRoot(t))

	foundSet := make(map[string]bool, len(found))
	var unlisted []string
	for _, f := range found {
		foundSet[f] = true
		if _, ok := legacyPRReaders[f]; !ok {
			unlisted = append(unlisted, f)
		}
	}
	var stale []string
	for f, tag := range legacyPRReaders {
		require.Contains(t, []string{readsSnapshot, readsInstanceData}, tag, "%s has an unknown tag", f)
		if !foundSet[f] {
			stale = append(stale, f)
		}
	}
	sort.Strings(stale)

	require.Empty(t, unlisted, "these files now name GitHubPRNumber/GitHubPRURL. A session can hold several PRs and these "+
		"scalars show only the active one: add a file to legacyPRReaders only after reading ActivePR (snapshot readers) "+
		"or the link-row query (instance-data readers), then tag it.")
	require.Empty(t, stale, "these allow-list entries no longer name the legacy fields; remove them")
}
