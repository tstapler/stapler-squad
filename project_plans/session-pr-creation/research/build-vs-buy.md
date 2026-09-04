# Build vs. Buy: session-pr-creation (Agent 6)

Scope note: nearly all backend machinery already exists —
`GitWorktree.CreatePR`/`findExistingPR` (`session/git/worktree_git.go:329,396`),
`headless.DraftPRDescription` (`session/headless/features.go:280`), and PR URL
persistence (`server/services/session_service.go:3690`). This document only
evaluates the remaining gap: a new mechanical-path RPC for non-backlog
sessions and its frontend modal, per `project_plans/session-pr-creation/requirements.md`.

## 1. Existing OSS library for PR creation/description (e.g. go-github vs. `gh` CLI)

**Option A — keep shelling out to `gh pr create` (current approach, unchanged)**

- Pros:
  - `checkGHCLI()` (`session/git/util.go:46`) already validates install + auth
    (`gh auth status`) before every call site — auth/config is a solved
    problem today, for free.
  - `gh` handles repo detection, remote resolution, default-branch
    resolution, and API versioning internally; none of that logic exists
    elsewhere in this codebase.
  - `CreatePR`/`findExistingPR` are already fully implemented, tested (see
    `session/git/worktree_git_test.go`), and reused by both the backlog path
    and (per this feature) the new mechanical RPC — zero new code for the
    GitHub-interaction layer itself.
  - No new dependency, no new auth flow (e.g. a PAT or GitHub App token)
    to provision, store, or rotate.
- Cons:
  - `.claude/rules/prefer-go-git-over-subshells.md` prefers native Go over
    subshells *when a Go library can do the job without re-implementing
    functionality the subshell gets for free*. That rule's own listed
    exception — "any operation needing a credential helper for push/fetch
    against a real remote" — applies directly here: `gh` already owns
    the user's GitHub credential/auth-flow, and `google/go-github` has no
    equivalent; it takes a bare API token and pushes the entire
    auth/config burden onto this codebase.
  - Subprocess overhead (fork/exec, text-parsed exit codes) per the rule's
    general concern — negligible for an RPC invoked at most once per PR
    click, not a hot path.
  - Every `gh` output-format quirk becomes this codebase's problem to work
    around, as `CreatePR`'s own comments show (`worktree_git.go:358-389`,
    the "some gh versions treat 'already exists' as success" and PR-number
    parsing fallback logic).

**Option B — `google/go-github` (or similar) for direct GitHub API calls**

- Pros:
  - Typed Go responses instead of parsing CLI stdout/JSON-jq output —
    removes the URL-regex/PR-number-parsing fallback chain in `CreatePR`.
  - No subprocess per call.
- Cons:
  - Not currently a dependency (`grep go-github go.mod go.sum` returns no
    matches) — net-new dependency for a gap this small.
  - Requires re-implementing everything `gh auth status` currently gets for
    free: token acquisition, storage, refresh, and the discovery of which
    account/keychain entry to use. This is exactly the "credential helper"
    exception `.claude/rules/prefer-go-git-over-subshells.md` calls out as a
    legitimate reason to keep shelling out.
  - Two competing auth surfaces (`gh`'s own vs. whatever this introduces)
    is worse than one, especially since backlog automation (out of scope
    for this feature, per requirements) keeps using `gh` regardless — a
    partial migration would leave the codebase authenticating to GitHub two
    different ways for the same overall product.
  - Would touch `findExistingPR`/`CreatePR`, both explicitly *not* part of
    this feature's scope ("Any change to the backlog automation
    (`pushAndCreatePR`) path itself" is out of scope, and the mechanical
    RPC is required to call these same functions directly per Acceptance
    Criterion 3) — rewriting them risks destabilizing a path already used
    in production by backlog automation.

**Verdict: Not recommended (go-github).** Keep the existing `gh`
CLI shell-out. The credential-helper exception in this repo's own
subshell-avoidance rule applies squarely to GitHub PR creation, the
existing code is tested and reused by two callers, and introducing
go-github would add a dependency and an auth surface to solve a problem
(text parsing of a single-purpose CLI call) that isn't actually causing
friction anywhere in the requirements. **Recommended: Option A**, i.e. do
nothing here — call `GitWorktree.CreatePR` directly from the new RPC as
Acceptance Criterion 3 already specifies.

## 2. SaaS/managed API for PR creation

N/A. GitHub's own REST/GraphQL API (reached today via the `gh` CLI) is
already the target system — there is no third-party PR-creation SaaS to
evaluate; "buy" and "the thing we already call" are the same system. No
reason to reconsider found during this pass — the repo is GitHub-hosted
(`origin`/`upstream-fanatics` remotes per root `CLAUDE.md`) and `gh` is
already a first-class dependency (`checkGHCLI`, existing Brewfile entries
elsewhere in the stapler-squad ecosystem).

## 3. LLM-generated PR body vs. deterministic template

Requirements' Acceptance Criterion 1 mandates the modal's initial body come
from `headless.DraftPRDescription` — "not a new prompt template" — so this
is scoped to a narrower question: **given the body is now user-editable in
a modal before the user confirms, is the extra LLM call still worth it
relative to always starting from `buildFallbackPRBody`
(`session/backlog_lifecycle.go:3442`)?**

- Pros of keeping `DraftPRDescription` as the default pre-fill:
  - Materially richer starting draft: it produces a Summary tied to *why*
    the change was made (via itemTitle/itemDescription context) plus a
    "What Changed" bullet list actually derived from the diff content —
    `buildFallbackPRBody` has no diff access at all; it only echoes the
    item's static description and acceptance criteria as a checklist, so
    it cannot describe what the diff actually contains.
  - The prompt has already been hardened against known failure modes found
    live in this repo (PRs #174/#175: conversational refusals, clarifying
    questions) via `prDescriptionSystemPrompt`'s explicit "no
    preamble/no clarifying questions" contract (`features.go:83-95`) — this
    isn't a naive first attempt, it's already had two production incidents
    burned into its constraints.
  - A human editing a draft that's already close is strictly less total
    work than a human writing a body from a bare template — even an
    imperfect LLM draft with the right sections saves the user from typing
    the Test Plan checklist and Summary from scratch.
  - The call is already a single short headless LLM call (`CallBlocking`),
    not a full agent turn — this is explicitly the "faster, cheaper"
    alternative the requirements doc contrasts against the `RunOneShot`
    agentic path (Baseline section, requirements.md:32-34). It does not
    reintroduce the cost problem this feature is trying to eliminate.
- Cons:
  - Adds one more network/LLM round-trip and its associated failure modes
    (timeout, pool exhaustion, empty-diff error per
    `DraftPRDescription`'s early return) to the "open modal" interaction —
    for a *manual* per-session action (not an automated backlog
    transition), a user is now waiting on an LLM call just to see a modal,
    where the deterministic path is instant.
  - Marginal value is genuinely lower here than in the backlog-automation
    case DraftPRDescription was built for: backlog automation has no human
    in the loop at PR-creation time, so the draft *is* the final quality
    bar. Here a human is about to read and likely edit the body anyway,
    which caps the cost of a mediocre draft (typos, a slightly-off Summary)
    at "the user fixes it," not "a bad PR ships unreviewed."
  - Failure handling gets slightly more complex in the modal flow: if
    `DraftPRDescription` errors (empty diff, pool call failure), the modal
    still needs *a* body to pre-fill — meaning the fallback template must
    be wired into this new path regardless, so the deterministic template
    is unavoidable infrastructure either way.
- Verdict: **Viable, but the fallback template must be first-class, not
  an afterthought.** Follow Acceptance Criterion 1 as written (LLM-drafted
  pre-fill by default) — the requirement already made this decision — but
  wire `buildFallbackPRBody`-equivalent logic as the explicit fallback when
  `DraftPRDescription` errors or returns empty, exactly as
  `pushAndCreatePR` already does (`backlog_lifecycle.go:3646-3655`), and
  do not block modal-open on the LLM call finishing if it can be
  reasonably async (e.g. open the modal immediately with a fallback/loading
  body, replace with the drafted body when the call resolves). This keeps
  the richer default while avoiding a hard UI stall on every "Create PR"
  click. A straight "always use the fallback template, drop the LLM call
  entirely" option is **Not recommended** — it would visibly regress body
  quality versus what backlog automation already produces for the same
  underlying function, for a marginal latency saving on a low-frequency,
  already-fast call.

## 4. Fork or adapt an existing closer-to-done implementation

Searched the current tree for any prior PR-creation modal/UI prototype:

- `grep -rl "PRModal|CreatePRModal|PrModal|pr-modal|PullRequestModal"` across
  `web-app/src` — **no matches**. No modal component exists to adapt.
- The only existing "Create PR" UI is the overflow-menu button in
  `web-app/src/components/sessions/SessionActionsOverflow.tsx:609-616`,
  which is a single button (not a modal) wired straight to the
  `RunOneShot` agentic RPC via `onRunOneShot` — this is the affordance
  Acceptance Criterion 7 requires removing/demoting, not something to
  extend in place.
- No RPC resembling "create PR from title/body/base-branch" exists yet in
  `server/services/session_service.go` — `RunOneShot` (line ~3616) is the
  only PR-adjacent entry point for non-backlog sessions, and it does not
  take structured title/body/base-branch fields at all (it takes a free-text
  prompt).
- Verdict: **Not recommended (nothing to fork).** No prior modal prototype,
  branch artifact, or partial RPC was found in the current tree to adapt.
  The new RPC and modal need to be built net-new, reusing
  `GitWorktree.CreatePR`/`findExistingPR` and `headless.DraftPRDescription`
  as their backing calls (already covered under points 1 and 3 above).

## Summary table

| # | Option | Verdict |
|---|--------|---------|
| 1 | Keep `gh pr create` CLI shell-out | **Recommended** |
| 1 | Migrate to `google/go-github` | Not recommended |
| 2 | Third-party SaaS PR API | N/A — GitHub API is already the target |
| 3 | LLM-drafted body (`DraftPRDescription`) as default pre-fill, with fallback-template safety net | **Viable / Recommended per AC1** |
| 3 | Always use deterministic fallback template only | Not recommended |
| 4 | Fork/adapt an existing modal or RPC prototype | Not recommended — none exists in current tree |
