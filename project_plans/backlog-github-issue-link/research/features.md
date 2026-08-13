# Research: Features — "link back to origin" precedent, URL edge cases, closing keywords, ID/URL drift

## 1. Existing "link back to origin" features — none render `ExternalID` today

`Grep -rn "ExternalID" session/ server/` shows `ExternalID` flowing through:
- `ExternalItem` (`session/backlog_plugin.go:22`) — populated by both plugins' `Fetch`.
- `BacklogItemData` (`session/repository.go:271`) and the ent schema/generated
  code (`session/ent/backlogitem.go`, `backlogitem_create.go`,
  `backlogitem_update.go`, `backlogitem/where.go`) — full ent CRUD/query
  support already exists for `ExternalID` (predicates like `ExternalIDContains`,
  `ExternalIDHasPrefix` etc. are all ent-generated boilerplate, not
  hand-written business logic).
- `GetBacklogItemByExternalID` (`session/ent_repository_backlog.go:466-481`) —
  used only by `SyncOne` to detect existing rows during sync, scoped by
  `sourceID` (see §4 below).
- `server/services/backlog_service.go:401` — the **only** place `ExternalID`
  crosses into the proto layer: `ExternalId: item.ExternalID` when building
  the gRPC response message.
- `proto/session/v1/backlog.proto:98` (`string external_id = 14;`) →
  generated `web-app/src/gen/session/v1/backlog_pb.ts:431` (`externalId: string`).

**Critically: `grep -rln "externalId" web-app/src` matches only the generated
`backlog_pb.ts` file.** No component under `web-app/src/components/backlog/*`
(`BacklogItemCard.tsx`, `BacklogItemDetail.tsx`, `BacklogItemPanel.tsx`,
`GitHubIssuePicker.tsx`, etc.) reads `externalId` or renders any GitHub
link/badge. The field is plumbed all the way to the frontend's generated
protobuf types and then **dropped on the floor** — dead data from the UI's
perspective.

**Consequence for this feature:** there is no existing URL-building
convention (e.g. constructing `https://github.com/{owner}/{repo}/issues/{id}`
client-side) to stay consistent with, because nothing builds or renders such
a URL anywhere in the codebase today. `GitHubIssuePicker.tsx` doesn't
construct GitHub URLs either — it only feeds owner/repo/id *into* the picker
UI, it doesn't render outbound links. This backend change is genuinely new
ground, not a convention-following exercise. It also means: once
`ExternalURL` is added to `BacklogItemData`/ent/proto, if a follow-up wants it
*visible in the web UI*, that's a fully separate, currently-nonexistent
component — out of scope for this backend-only requirements doc, but worth
flagging as a natural next step (not in the ACs, don't build it).

## 2. `net/url` vs string-prefix idiom precedent

The codebase has **two live `net/url` users**, both server-side and both for
security/routing-correctness reasons, not casual string checks:
- `server/services/ws_stream_bridge.go:51` — `url.Parse(r.URL.String())` to
  rebuild a stripped-prefix path for proxying.
- `server/services/connectrpc_websocket.go:83` — `url.Parse(origin)` then
  `.Hostname()` for **CORS origin allowlisting** — deliberately uses
  `Hostname()` rather than `strings.Contains`/`HasPrefix` because origin
  spoofing via substring tricks is a real attack vector there.

By contrast, the GitHub plugins' owner/repo config (`session/backlog_plugin_github.go:30-31`,
`session/backlog_plugin_github_prs.go:24-25`) is **not** parsed out of a URL
at all — `Owner`/`Repo` come directly from plugin JSON config fields, never
by parsing `HTMLURL`. So there is no existing "parse a GitHub URL into
owner/repo/kind" helper anywhere to reuse or mimic.

**Recommendation for `closingKeywordFor`:** the AC's own wording ("whether
the URL contains `/issues/` or `/pull/`") is a plain substring check, and
given no `net/url`-based GitHub-URL parser exists to imitate, a simple
`strings.Contains(url, "/issues/")` / `strings.Contains(url, "/pull/")` is
both spec-compliant and consistent with the codebase's general pattern of
reserving `net/url` for cases with actual security/correctness stakes (CORS,
proxy path rewriting) rather than for informational logic like this. Using
`net/url.Parse` purely to inspect `.Path` would be marginally more robust
(protects against a `/pull/`-in-querystring false positive) but is arguably
over-engineering for a same-origin, self-generated `html_url` value that
GitHub always returns in the canonical `https://github.com/{owner}/{repo}/issues/{n}`
or `.../pull/{n}` shape.

### Edge cases `closingKeywordFor` should handle beyond the two documented cases

| Input | Risk if unhandled | Suggested behavior |
|---|---|---|
| `""` (empty ExternalURL) | Caller in `BuildSessionInitialPrompt` already gates the whole section on `ExternalURL != ""` per AC4 — `closingKeywordFor` itself should still not panic if called with `""` (defensive default). | Return `"Related: "` (safe default) or empty string; document that callers must gate on non-empty URL first. |
| Malformed URL (not a real URL at all, e.g. free text) | `strings.Contains` won't error, just won't match either substring → falls through to default. | Fall through to a default keyword (`"Related: "`) rather than erroring — this is prompt text, not something that should ever fail a build. |
| Non-GitHub URL (e.g. a GitLab/Jira URL that happens to contain `/issues/`) | Both GitLab and Jira use `/issues/` in their URL shapes too — a naive `strings.Contains(url, "/issues/")` would misfire "Fixes " for a GitLab issue URL, which is *actually correct* behavior (GitLab also honors `Fixes/Closes` keywords), but would be wrong for a Jira URL (Jira doesn't auto-close via commit messages the same way, though many integrations do support `Fixes JIRA-123`-style syntax). Since the only two plugins that populate `ExternalURL` today are GitHub Issues/PRs (`session/backlog_plugin_github.go`, `session/backlog_plugin_github_prs.go`), this is currently moot, but worth a code comment noting the heuristic is GitHub-shape-specific. |
| Trailing slash (`.../issues/42/`) | `strings.Contains` still matches — no issue. | No special handling needed. |
| Query string/fragment (`.../pull/42?diff=split#discussion_r123`) | `strings.Contains` still matches on `/pull/` regardless of trailing query/fragment — no issue since the substring appears before the query fragment. | No special handling needed. |
| Both `/issues/` and `/pull/` somehow present (theoretically impossible from GitHub's real URL shapes, but defensive coding) | Ambiguous precedence. | Check `/pull/` and `/issues/` as mutually exclusive `if/else if`; document priority order (doesn't matter in practice since a real GitHub URL is one or the other, never both). |

Given the two real producers (`GitHubIssuesPlugin`, `GitHubPRsPlugin`)
guarantee well-formed `issue.HTMLURL`/`pr.HTMLURL` values, the pragmatic
scope is: **handle empty string and "neither substring matches" by falling
back to a safe default ("Related: ")**, and don't over-invest in parsing
robustness the current callers can't produce.

## 3. GitHub closing keywords — spec-compliant literal

GitHub's own docs ("Linking a pull request to an issue using a keyword")
recognize these keywords (case-insensitive) for auto-closing an issue when
merged to the default branch: `close`, `closes`, `closed`, `fix`, `fixes`,
`fixed`, `resolve`, `resolves`, `resolved`. Any of these prefixed to
`#123` or a full issue URL in a PR body/commit message triggers auto-close.

- **"Fixes "** (capitalized, with trailing space before the URL) is the
  conventional, most commonly recommended choice for **bug-fix-shaped** work
  — matches the AC's own literal ("Fixes " for `/issues/` URLs). No reason to
  deviate; it's spec-compliant and idiomatic.
- **"Closes "** and **"Resolves "** are equally spec-compliant synonyms;
  there's no functional difference in GitHub's auto-close behavior. Since the
  AC already pins the literal to `"Fixes "`, there's no open decision here —
  just confirming it's not a made-up keyword.
- **PRs cannot "close" other PRs via these keywords** — the keyword syntax
  only auto-closes *issues*, not other PRs. This is exactly why the AC
  specifies `"Related: "` (a plain cross-reference, not a closing keyword)
  for `/pull/` URLs — linking one PR to another PR via `Fixes`/`Closes` would
  either silently no-op or (per GitHub's actual behavior) do nothing at all
  since PRs aren't issues in the closing-keyword sense. The AC's choice here
  is technically correct.

## 4. `GetBacklogItemByExternalID` scoping and ID/URL drift risk

`GetBacklogItemByExternalID` (`session/ent_repository_backlog.go:466-481`) is
already scoped by `sourceID` (doc comment explicitly warns: "External IDs
... are only unique within their source ... this must never match across
sources"). Adding `ExternalURL` as a sibling field does **not** change this
lookup's semantics — `SyncOne` (`session/backlog_sync.go:241`) will keep
looking up by `(source.ID, extItem.ExternalID)` only; `ExternalURL` is purely
along for the ride once a match is found, never a lookup key. No new
collision risk is introduced by adding the field, and no scoping change is
needed to `GetBacklogItemByExternalID` itself.

**Drift risk (repo rename changes URL but not ID):** yes, this is real and
already implicitly acknowledged by AC6's "known limitation" framing. If a
GitHub repo is renamed, `issue.Number` (`ExternalID`) is stable (GitHub issue
numbers don't change), but `issue.HTMLURL` **does** change to reflect the new
repo slug (GitHub's `html_url` always reflects the *current* canonical
path — old URLs redirect but the API returns the current slug). Two
consequences worth flagging in the plan/pre-mortem, though neither is a
blocker for this requirements-locked scope:

1. `SyncOne`'s unconditional `ExternalURL` backfill (AC6) will naturally
   self-heal this on every sync — since every `state=open` `Fetch` re-reads
   the current `HTMLURL` from GitHub's API each cycle, a stale `ExternalURL`
   from before a rename gets overwritten the next successful sync. This is a
   feature of the "unconditional, bypass local-wins" design, not something
   that needs extra code.
   2. The **explicitly out-of-scope known limitation** (already called out in
   requirements.md: "Backfilling `ExternalURL` for items whose source
   issue/PR has since been closed and no longer appears in the plugin's
   `state=open` Fetch") compounds with rename drift: a *closed* issue in a
   *renamed* repo will never get backfilled and could carry a stale URL
   indefinitely if it was created before the rename and closed before ever
   syncing post-rename. This is a pre-existing accepted limitation per the
   requirements doc (§ "Explicitly out of scope"), not a new risk introduced
   by this feature — just worth a one-line mention in the plan's known-limitations
   section so it doesn't get "discovered" later as a surprise bug.

No repository/lookup-method changes are needed beyond what AC1/AC6 already
specify (add the field, backfill unconditionally in the existing update
path).

## Summary of implications for planning

- No existing GitHub-URL-parsing helper or `net/url`-based idiom to reuse —
  `closingKeywordFor` is new code; a plain `strings.Contains` check
  (matching the AC's own wording) is the right level of engineering, with a
  documented fallback default ("Related: ") for empty/unmatched input rather
  than a panic or error return.
- No web UI convention exists for rendering `ExternalID`/`ExternalURL` —
  `externalId` is currently dead data past the generated proto types on the
  frontend, so this backend change has no frontend consistency constraint to
  satisfy (and shouldn't attempt to add frontend rendering — out of scope).
- `"Fixes "` / `"Related: "` are both spec-compliant per GitHub's documented
  closing-keyword list; no substitution needed.
- `GetBacklogItemByExternalID`'s existing per-source scoping is unaffected by
  adding `ExternalURL`; the only real drift risk (repo rename → stale URL on
  closed issues) is already covered by the requirements doc's accepted
  "known limitation," not a new gap this research uncovered.
