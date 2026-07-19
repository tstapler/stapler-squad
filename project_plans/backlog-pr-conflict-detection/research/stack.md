# Stack Research: PR Mergeable-State Detection (`gh pr view` fields)

Agent: Stack (Research Phase, backlog-pr-conflict-detection)

## 1. Current implementation (baseline for minimal diff)

`session/git/worktree_git.go:326-438` — `PRStatus` struct and `GetPRStatus(prNumber int)`:

```go
type PRStatus struct {
    CIFailing          bool
    HasBlockingReviews bool
    FeedbackText       string
}

cmd := safeexec.CommandContext(ctx, "gh", "pr", "view", strconv.Itoa(prNumber),
    "--json", "statusCheckRollup,reviews,comments")
```

- 30s context timeout, invoked with `cmd.Dir = g.worktreePath`.
- `checkGHCLI()` (`session/git/util.go:45-61`) is called first — verifies `gh` is on `PATH` and `gh auth status` succeeds. No version check is performed anywhere; this repo already treats `gh` as a required, pre-authenticated dependency.
- Response is unmarshaled into an anonymous struct with only `statusCheckRollup`, `reviews`, `comments` fields — `mergeable`/`mergeStateStatus` are not requested or parsed today.
- `FeedbackText` is built by string concatenation as each signal is evaluated (failing CI checks section, then review-changes-requested section, then general comments section) — a conflict section would slot into this same pattern between the CI and review sections, or after them.

**Minimal diff**: add `mergeable,mergeStateStatus` to the existing `--json` flag string (comma-separated, no new `gh` invocation), add two string fields to the anonymous unmarshal struct, add a `PRConflicting bool` (or similarly named) field to `PRStatus`, and add one more evaluation block that mirrors the existing CI/review blocks' shape (checks a value, sets a bool, appends a `FeedbackText` section). No new dependency, no new process spawn, no new timeout tuning needed — same `gh pr view` call, same 30s budget.

## 2. Field semantics — confirmed live against this repo's `gh` (v2.86.0)

Ran `gh pr view --json mergeable,mergeStateStatus --help` locally, plus `gh pr view <badfield> --json badfield` (which lists all valid fields on error) — both `mergeable` and `mergeStateStatus` are present in the current CLI's field list, alongside the fields already used (`statusCheckRollup`, `reviews`, `comments`) and `reviewDecision` (not currently used, but simpler than the current per-review loop if the field-list approach is ever revisited — out of scope here).

Live test against two real open PRs in this repo:

```
$ gh pr view 151 --json mergeable,mergeStateStatus,number
{"mergeStateStatus":"CLEAN","mergeable":"MERGEABLE","number":151}
$ gh pr view 148 --json mergeable,mergeStateStatus,number
{"mergeStateStatus":"CLEAN","mergeable":"MERGEABLE","number":148}
```

Both fields are returned as **JSON strings** (enum names), not booleans — parse as `string`, same pattern as the existing `check.Conclusion`/`check.State` string fields, which the current code already uppercases and compares with `==` (`worktree_git.go:393-396`). Follow that exact convention for consistency.

### `mergeable` — GraphQL `MergeableState` enum (backs `gh pr view --json mergeable`)

Per GitHub's GraphQL schema reference (`docs.github.com/en/graphql/reference/enums#mergeablestate`) and confirmed against community reports (cli/cli discussions/issues, cited below):

| Value | Meaning |
|---|---|
| `MERGEABLE` | The pull request can be merged cleanly against its base. |
| `CONFLICTING` | The pull request has merge conflicts and cannot be merged as-is. |
| `UNKNOWN` | GitHub has not finished computing mergeability yet (async — see §4). |

### `mergeStateStatus` — GraphQL `MergeStateStatus` enum (more granular; backs `gh pr view --json mergeStateStatus`)

| Value | Meaning |
|---|---|
| `BEHIND` | The head branch is out of date with the base branch (no conflict necessarily — just needs updating). |
| `BLOCKED` | Merge is blocked by something other than conflicts — most commonly unmet required-status-checks or required-review policy, not necessarily a code conflict. |
| `CLEAN` | Mergeable and passing commit status — the healthy case (both live PRs above returned this). |
| `DIRTY` | The merge commit cannot be cleanly created — i.e. real merge conflicts. This is the direct signal this project needs. |
| `DRAFT` | Merge is blocked solely because the PR is a draft. |
| `HAS_HOOKS` | Mergeable, passing commit status, and pre-receive hooks configured (rare; GitHub Enterprise feature). |
| `UNKNOWN` | State not yet computed (async — same caveat as `mergeable: UNKNOWN`). |
| `UNSTABLE` | Mergeable but commit status is not fully passing (e.g. CI still running, or a non-required check failed). |

**Practical implication for this project**: `mergeStateStatus == "DIRTY"` (or `mergeable == "CONFLICTING"`) is the precise conflict signal. `BLOCKED`/`UNSTABLE`/`BEHIND` are **not** conflicts — `BLOCKED` in particular is often a required-check/required-review gate, which would already be surfaced (or not) by the existing `CIFailing`/`HasBlockingReviews` logic; treating `BLOCKED` as a conflict would risk double-triggering or false-positive conflict spawns for PRs that are actually failing CI (already handled) or awaiting a required approval (not the same as a merge conflict). Recommend keying the new `PRConflicting` signal specifically off `mergeStateStatus == "DIRTY"` (most precise, matches the `.gitignore`-corruption conflicts described in the requirements doc), with `mergeable == "CONFLICTING"` as a fallback/cross-check since both fields are being fetched for free in the same call.

## 3. `gh` CLI version considerations

- No version gate needed. Both fields have shipped in `gh pr view --json` for a long time; the fields list embedded in the installed 2.86.0 binary's `--help` output already includes them, and this repo's `checkGHCLI()` performs no version pinning today — it only checks `PATH` presence and `gh auth status`. Adding these fields does not raise the minimum supported `gh` version.
- One open compatibility risk found via search, not observed locally: cli/cli issue [#9583](https://github.com/cli/cli/issues/9583) ("Output of `gh pr view PR --json mergeable` incorrect", filed against gh v2.54.0, July 2024) reports `mergeable` returning `MERGEABLE` while the REST API's `mergeable_state` showed `blocked` for the same PR. The issue was inconclusive on root cause (marked "needs-investigation"); no fix/changelog entry was found confirming resolution. This is a reason to prefer `mergeStateStatus` (`DIRTY`) as the primary signal over `mergeable` (`CONFLICTING`) alone — `mergeStateStatus` is the more granular field and less likely to be the one affected by whatever staleness/mapping bug #9583 describes, though we have no direct confirmation either way. Belt-and-suspenders: treat conflict as `mergeStateStatus == "DIRTY" || mergeable == "CONFLICTING"`.

## 4. Async computation / `UNKNOWN` handling — recommendation

GitHub computes both `mergeable` and `mergeStateStatus` **asynchronously**, server-side, particularly after: a push to the head branch, a push to the base branch, or a PR being freshly opened. Immediately after such an event, `gh pr view` can return `mergeable: "UNKNOWN"` / `mergeStateStatus: "UNKNOWN"` for a short window (typically single-digit seconds, but can be longer for large repos/branches under GitHub's own docs and community reports) until GitHub finishes the background computation.

Evidence:
- GitHub's GraphQL docs describe `UNKNOWN` on `MergeableState` as: "The mergeability of the pull request is still being calculated" (per cli/cli discussion #8020, which quotes GitHub's own docs verbatim).
- cli/cli discussion [#8020](https://github.com/cli/cli/discussions/8020) ("what is UNKNOWN in mergeable") — a maintainer confirms this recalculates whenever the base branch changes, and for PRs against frequently-updated base branches, `UNKNOWN` can recur intermittently, not just once at PR creation.
- No official GitHub or `gh` documentation was found recommending a specific retry count/backoff; the consistent guidance across sources is "wait, it resolves on its own," not "poll aggressively."

**Recommendation for `ReconcilePRPending`** (matches the Rabbit Hole flagged in requirements.md, "Mergeable-state polling lag"):
- Treat `UNKNOWN` as **not conflicting** for that reconciliation cycle — i.e. do not spawn a fix session — and let the next poll cycle re-check. This is the conservative, false-positive-avoiding choice: `ReconcilePRPending` already runs on a recurring poll interval (per requirements.md's existing architecture), so an `UNKNOWN` this cycle self-heals within one more interval in the overwhelmingly common case, with no special retry/backoff logic needed in this function — the existing poll loop *is* the retry mechanism.
- Do not add a dedicated sleep-and-recheck-in-place retry inside `GetPRStatus` itself — that would add latency to every call (most calls return a definitive state immediately, as seen in both live test PRs above) for a transient condition the outer poll loop already handles for free.
- This mirrors how `CIFailing` already behaves for in-progress (non-terminal) checks: the current code only sets `CIFailing = true` for terminal conclusions (`FAILURE`, `TIMED_OUT`, `CANCELLED`) and implicitly treats `PENDING`/`IN_PROGRESS` as "not yet failing, check again later" — the same "unresolved state defers to next poll" pattern should apply to `mergeStateStatus: UNKNOWN`.

## 5. Summary of recommended code shape (for Phase 3 planning, not implemented here)

```go
cmd := safeexec.CommandContext(ctx, "gh", "pr", "view", strconv.Itoa(prNumber),
    "--json", "statusCheckRollup,reviews,comments,mergeable,mergeStateStatus")

var payload struct {
    // ... existing fields unchanged ...
    Mergeable       string `json:"mergeable"`
    MergeStateStatus string `json:"mergeStateStatus"`
}

// Evaluate conflict state (skip UNKNOWN — defer to next poll cycle).
mss := strings.ToUpper(payload.MergeStateStatus)
mg := strings.ToUpper(payload.Mergeable)
if mss == "DIRTY" || mg == "CONFLICTING" {
    status.PRConflicting = true
    sb.WriteString("## Merge conflict\nThis PR's branch has merge conflicts against its base and cannot be merged as-is. Rebase onto the base branch and resolve conflicts; this is not necessarily a re-implementation of the original acceptance criteria.\n\n")
}
// mss == "UNKNOWN" or mg == "UNKNOWN": leave PRConflicting false, no log noise — expected transient state, next poll will resolve it.
```

## Sources

- [GitHub GraphQL API — Enums reference](https://docs.github.com/en/graphql/reference/enums) (index; specific `MergeableState`/`MergeStateStatus` value tables per public GitHub GraphQL schema)
- [cli/cli discussion #8020 — "what is UNKNOWN in mergeable"](https://github.com/cli/cli/discussions/8020)
- [cli/cli issue #9583 — "Output of `gh pr view PR --json mergeable` incorrect"](https://github.com/cli/cli/issues/9583)
- [cli/cli issue #13239 — "gh search prs --json missing mergeStateStatus and reviewDecision fields"](https://github.com/cli/cli/issues/13239)
- [gh pr view manual](https://cli.github.com/manual/gh_pr_view)
- Local verification: `gh --version` → 2.86.0 (2026-01-21); `gh pr view --json <badfield>` field-list error output; live `gh pr view 151/148 --json mergeable,mergeStateStatus,number` against this repo's own open PRs.
- `session/git/worktree_git.go:326-438` (`PRStatus`, `GetPRStatus`) and `session/git/util.go:45-61` (`checkGHCLI`) — read directly, this repo.
