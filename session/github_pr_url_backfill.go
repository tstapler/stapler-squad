package session

// github_pr_url_backfill.go — one-time idempotent migration that populates
// Session.github_pr_url for rows that have a known github_pr_number/owner/repo
// but an empty URL. This gap predates the fix in
// server/services/session_service.go's CreateSession handler, which now
// builds the URL from the parsed GitHubRef at creation time — this migration
// covers every row written before that fix, across every workspace DB, since
// each is fixed up here on the next process startup rather than requiring a
// manual per-row SQL edit.
//
// Host recovery is best-effort: the row itself only stores owner/repo, not
// host, so the host is recovered from the session's persisted local path via
// hostFromClonedPath (see repo_path.go) wherever that path still encodes the
// GOPATH-style <host>/<owner>/<repo> convention. Rows where host can't be
// confidently recovered (e.g. a plain local-directory session with no host
// segment in its path) are skipped rather than guessed — defaulting to
// "github.com" would silently produce a wrong URL for GitHub Enterprise rows.

import (
	"context"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent/session"
)

// runGitHubPRURLBackfill re-saves every Session that has a known PR number
// and owner/repo but no github_pr_url, populating the URL when the host can
// be recovered from the session's local path. Idempotent: rows that already
// have a URL (including freshly-created ones) are excluded by the query
// itself. Safe to call on a fresh/empty database. Best-effort per row: a
// single row's save failure (or unrecoverable host) is logged and does not
// abort the rest.
func runGitHubPRURLBackfill(ctx context.Context, er *EntRepository) error {
	rows, err := er.client.Session.Query().
		Where(
			session.GithubPrNumberGT(0),
			session.Or(session.GithubPrURLEQ(""), session.GithubPrURLIsNil()),
			session.GithubOwnerNEQ(""),
			session.GithubRepoNEQ(""),
		).
		All(ctx)
	if err != nil {
		// Table may not exist yet (fresh DB before schema.Create) — ignore,
		// mirroring runStatusRemap's same defensive posture.
		return nil //nolint:nilerr
	}

	var migrated, skipped int
	for _, row := range rows {
		ref, err := github.NewRepoRef(row.GithubOwner, row.GithubRepo)
		if err != nil {
			skipped++
			continue
		}
		path := row.Path
		if path == "" {
			path = row.WorkingDir
		}
		host := hostFromClonedPath(path, ref)
		if host == "" {
			skipped++
			continue
		}
		url := (&GitHubRef{Host: host, Owner: ref.Owner(), Repo: ref.Repo(), PRNumber: row.GithubPrNumber}).PRURL()
		if _, saveErr := er.client.Session.UpdateOneID(row.ID).
			SetGithubPrURL(url).
			Save(ctx); saveErr != nil {
			log.WarningLog.Printf("[Migration] github pr url backfill: session=%d: %v", row.ID, saveErr)
			continue
		}
		migrated++
	}
	if migrated > 0 {
		log.InfoLog.Printf("[Migration] github pr url backfill: populated %d row(s)", migrated)
	}
	if skipped > 0 {
		log.InfoLog.Printf("[Migration] github pr url backfill: skipped %d row(s) with unrecoverable host", skipped)
	}
	return nil
}
