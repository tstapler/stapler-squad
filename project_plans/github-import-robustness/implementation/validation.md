# Validation Notes: github-import-robustness

## Adversarial Review Pass

### Finding 1 — Rate limit from DB lookup in ListGitHubIssues
**Risk level**: Low  
**Assessment**: The `GetBacklogItemsByExternalURLs` call is in-process SQLite (no network), bounded by page size (≤100 issues). A single IN-clause query. No meaningful latency or rate-limit concern. **Accepted**.

### Finding 2 — AddIssueLabel silent failure
**Risk level**: Medium  
**Assessment**: Tokens configured in the GitHub keychain may only have `read:org` or `public_repo` scope, not full `repo` write. `AddIssueLabel` failures are best-effort and logged at Warn. The import itself must not fail. This is acceptable because the primary dedup mechanism (DB lookup in AC-1) does not depend on the label. **Accepted with mitigation**: log the exact HTTP status code on failure to aid debugging.

### Finding 3 — Archived items have their GitHub issue closed
**Risk level**: Low-Medium  
**Assessment**: A user who archives an item speculatively could end up with a closed GitHub issue. Counter-argument: leaving an issue open after the work is abandoned is a worse default. Mitigation: the bot comment for archived transitions explicitly says "Closed automatically — the linked backlog item was archived in stapler-squad. Reopen the issue if this was in error." **Accepted**.

### Finding 4 — Source-less CloseIssue requires token with write scope
**Risk level**: Low  
**Assessment**: The keychain token for github.com may be read-only for some users. If `CloseIssue` fails for a source-less item, the error is logged at Warn (same as existing forward-sync failure path). **Accepted** — no regression from current behavior (current: silently skipped; new: attempted and logged on failure).

### Finding 5 — external_url index migration
**Risk level**: Low  
**Assessment**: Adding an index to `session/ent/schema/backlogitem.go` triggers `make ent-gen` and a SQLite migration on next startup. The generated migration is a single `CREATE INDEX IF NOT EXISTS`. Zero data migration. **Accepted**.

### Finding 6 — RemoveIssueLabel on archive races with forward-sync close
**Risk level**: Low  
**Assessment**: The forward-sync subscriber closes the issue AND removes the claim label in the same handler path for archived transitions. If `CloseIssue` succeeds but `RemoveIssueLabel` fails (or vice versa), the state is slightly inconsistent. Since both operations are best-effort and the issue is closed regardless, the inconsistency is cosmetic. **Accepted**.

## Open Questions

1. **Should `claim_label` filtering in `ListGitHubIssues` call the GitHub API to check labels, or only use the labels returned in the list response?**  
   GitHub's `GET /repos/{owner}/{repo}/issues` returns labels per issue in the response. No extra API call needed — filter in-memory on the `labels` field. ✅

2. **What if the configured `claim_label` does not exist in the GitHub repo?**  
   `AddIssueLabel` will receive a 422 from GitHub ("Label does not exist"). Handle: attempt to create the label first via `POST /repos/{owner}/{repo}/labels` with a default color, then retry. Or: just log Warn and skip — label creation is a separate UX concern. **Decision**: Log Warn and skip label application; document that the label must be pre-created in the repo (or created manually on first use). Follow-up: add a settings UI hint.

3. **For AC-3 (claim_label filter in ListGitHubIssues), should it use the source's configured claim_label or the global default?**  
   Use the source's `claim_label` if an ItemSource is configured for the repo; fall back to `"stapler-squad"` globally. For the `ListGitHubIssues` RPC, the request already has `owner/repo`; look up matching sources. If none, use default.

## Validation Checklist

- [ ] `make proto-gen` succeeds after adding `already_in_backlog` field
- [ ] `make ent-gen` succeeds after adding `external_url` index
- [ ] `make build` succeeds
- [ ] `make test` passes (especially `TestListGitHubIssues_*`, `TestForwardSyncSubscriber_*`)
- [ ] Manual: import an issue → verify `stapler-squad` label appears on GitHub
- [ ] Manual: mark item done → verify GitHub issue is closed
- [ ] Manual: archive item → verify GitHub issue is closed, label removed
- [ ] Manual: open import picker → already-imported issues are hidden by default
- [ ] Manual: toggle "Show already imported" → hidden issues appear grayed out
