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

// Backlog-side twin of legacy_pr_readers_test.go. An item stores one PR in the
// scalars PrNumber / PrURL, but its sessions can now hold several. A file that
// names the scalars must record why that is still right.
const (
	// itemPRsSeam: reads the item's PR set through the PRSetReader seam.
	itemPRsSeam = "via-itemPRs"
	// itemScalarOK: the single stored PR is what this code wants; the reason says why.
	itemScalarOK = "scalar-ok"
)

type itemPRReader struct{ disposition, reason string }

// legacyItemPRReaders lists every non-test Go file that names the item scalars.
// No file reads the set yet, so each is scalar-ok today; a file moves to
// itemPRsSeam when its reads go through itemPRs.
var legacyItemPRReaders = map[string]itemPRReader{
	"session/ent_repository_flow_markers.go":                     {itemScalarOK, "flow-marker rows carry their own PR number (analytics), not the item scalar"},
	"session/ent_repository_session_pr_guard.go":                 {itemScalarOK, "guard view flags an item scalar no link or tombstone represents, the same rule as the backfill"},
	"server/mcp/tools_backlog_pr.go":                             {itemScalarOK, "pr_pending reassignment guard compares the one tracked PR number"},
	"server/mcp/tools_stuck_report.go":                           {itemPRsSeam, "report lists the whole item PR set under reader:backlog; the CI cross-check still targets the tracked scalar"},
	"server/services/backlog_debug_seed_handler.go":              {itemScalarOK, "debug seed writes the scalars onto a fixture item"},
	"server/services/backlog_github_forward_sync.go":             {itemScalarOK, "close comment links the item's tracked PR URL"},
	"server/services/backlog_github_progress_sync.go":            {itemPRsSeam, "review comment lists the whole item PR set under reader:backlog; the tracked URL is the flag-off fallback"},
	"server/services/backlogsvc/backlog_service.go":              {itemScalarOK, "maps the stored scalars onto the item proto"},
	"server/services/backlogsvc/backlog_service_lifecycle.go":    {itemScalarOK, "request validation for the explicit PR link/reassign RPC"},
	"server/services/backlogsvc/backlog_service_pr_fix_steer.go": {itemScalarOK, "derives the steering link key from the tracked PR URL"},
	"server/services/backlogsvc/backlog_service_ship.go":         {itemScalarOK, "ship refuses an item that already tracks a PR"},
	"server/services/backlogsvc/backlog_service_ship_status.go":  {itemScalarOK, "reports the tracked PR URL in the ship status"},
	"server/services/backlogsvc/backlog_service_stuck.go":        {itemScalarOK, "maps stored rows onto the stuck-item proto"},
	"server/services/backlogsvc/backlog_service_triage.go":       {itemScalarOK, "PR-fix prompt text names the tracked PR"},
	"server/services/pr_creation_service.go":                     {itemScalarOK, "fills the CreatePullRequest response proto, not an item"},
	"session/backlog_item_pr_mirror.go":                          {itemScalarOK, "writes the scalar mirror from ActivePR of the set; the set never reads it back"},
	"session/backlog_item_prs.go":                                {itemPRsSeam, "the seam itself: the scalar is only the unrepresented-legacy fallback input"},
	"session/backlog/backlog_lifecycle.go":                       {itemScalarOK, "merge check on the item's tracked PR number"},
	"session/backlog/backlog_lifecycle_pr.go":                    {itemScalarOK, "pr_pending reconciler is keyed on the item's tracked PR"},
	"session/domain/backlog.go":                                  {itemScalarOK, "domain field definitions and their doc comments"},
	"session/domain/backlogdto/backlog_item.go":                  {itemScalarOK, "backlog item DTOs carrying the stored columns"},
	"session/domain/backlogdto/stuck.go":                         {itemScalarOK, "stuck-state row DTO carrying the stored columns"},
	"session/ent_repository_backlog.go":                          {itemScalarOK, "persistence mapping of the stored columns"},
	"session/session_pr_backfill.go":                             {itemScalarOK, "backfill imports the stored scalar into links"},
	"session/storage.go":                                         {itemScalarOK, "PR reassignment guard compares the stored scalar"},
	"session/storage_backlog.go":                                 {itemScalarOK, "storage helpers derive a number from the stored URL"},
}

var legacyItemPRFieldRe = regexp.MustCompile(`\b(PrNumber|PrURL)\b`)

func filesNamingItemPRScalars(t *testing.T, root string) []string {
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
		if legacyItemPRFieldRe.Match(src) {
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	})
	require.NoError(t, err)
	sort.Strings(found)
	return found
}

func TestLegacyItemPRReaders_ShouldMatchAllowList_WhenGreppingPrNumberPrURL(t *testing.T) {
	t.Parallel()
	found := filesNamingItemPRScalars(t, moduleRoot(t))

	foundSet := make(map[string]bool, len(found))
	var unlisted []string
	for _, f := range found {
		foundSet[f] = true
		if _, ok := legacyItemPRReaders[f]; !ok {
			unlisted = append(unlisted, f)
		}
	}
	var stale, malformed []string
	for f, r := range legacyItemPRReaders {
		if r.disposition != itemPRsSeam && r.disposition != itemScalarOK {
			malformed = append(malformed, f+": unknown disposition "+r.disposition)
		}
		if r.reason == "" {
			malformed = append(malformed, f+": missing reason")
		}
		if !foundSet[f] {
			stale = append(stale, f)
		}
	}
	sort.Strings(stale)
	sort.Strings(malformed)

	require.Empty(t, unlisted, "these files now name the item scalars PrNumber/PrURL, which show one PR while an item's "+
		"sessions can hold several: add a file to legacyItemPRReaders only with a disposition (via-itemPRs, or scalar-ok with a reason).")
	require.Empty(t, stale, "these allow-list entries no longer name the item scalars; remove them")
	require.Empty(t, malformed)
}
