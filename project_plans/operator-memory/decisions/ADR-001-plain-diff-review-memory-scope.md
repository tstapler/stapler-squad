# ADR-001: Plain-diff review path gets memory on a best-effort basis, not guaranteed every call

## Status

Accepted

## Context

Operator memory (`OPERATOR.md`/`REPO.md`) is meant to be assembled fresh and
appended to the system prompt on every headless triage/review call
(requirements.md, functional requirement 3). Whether that guarantee actually
holds depends on which of three call paths a given headless call takes,
confirmed by reading `session/headless/pool.go` and `session/headless/caller.go`
(`project_plans/operator-memory/research/architecture.md`, "Summary of the key
finding"):

| Call site | `CallOptions.WorkDir` | Pool behavior | Fresh system prompt every call? |
|---|---|---|---|
| `TriggerTriage` (`server/services/backlog_service_triage.go:2330-2348`) | always set (`itemRepoPath`) | throwaway `oneShot` Pool, `MaxCallsPerSession: 1` | Yes, always |
| `TriggerReReview` empty-diff / codebase-read path (`session.BuildReviewCallOptions`, `diff == ""`) | set (`codebaseWorkDir`) | same `oneShot` bypass | Yes, always |
| `TriggerReReview` plain-diff path (`session.BuildReviewCallOptions`, `diff != ""`) | unset (`headless.CallOptions{}`) | real claude-CLI session reuse via `acquireSession`/`--resume`, keyed by `FeatureKey` alone (not per-repo), rotating after `MaxCallsPerSession` (25) calls or 3 consecutive errors | **No** — `--system-prompt` is only sent on the first call of a rotation window; every resumed call silently drops whatever `systemPrompt` string the caller passed |

Triage and the codebase-read review path are one-shot subprocesses per call —
"load memory fresh, append it, done" is correct and complete there. The
plain-diff review path is the one place where "load once per headless call"
(what this feature does) and "the running claude-CLI session actually
receives a fresh system prompt" (what the Pool does) diverge: a memory update,
or even a different item whose repo differs from the one baked into the
currently-resumed session, will not reach the model until that session's next
rotation — which is not scoped to a single repo or a single backlog item.

Two options were identified (architecture.md, Q section "Summary of the key
finding"):

- **Option A** — accept it as a documented limitation. Memory on the
  plain-diff review path is "eventually applied, on session rotation" rather
  than "every call," and can carry a stale or wrong-repo snapshot mid-window.
- **Option B** — key `Pool.sessions` by `(FeatureKey, repoPath)` instead of
  `FeatureKey` alone, or force rotation when a hash of the assembled
  `systemPrompt` changes since the pool's last call for that key. Either
  fixes the correctness gap but is a change to shared `Pool` infrastructure
  well beyond "storage + injection layer" — the scope this backlog item
  (`2da5cd02-5b45-4f75-a053-b33bbb3e3792`) is bounded to.

## Decision

**Option A.** Ship the storage + injection layer as scoped, and let the
plain-diff review path inherit the Pool's existing session-reuse semantics
as-is. `session.BuildReviewCallOptions` still calls
`opmemory.LoadSnapshot(repoPath)` and passes the result into
`headless.HeadlessReviewSystemPrompt(memorySnapshot)` on every invocation —
the caller-side contract ("load fresh, every call") is honored uniformly
across all three paths. What is *not* fixed by this item is the Pool's
internal decision to drop `--system-prompt` on a resumed call; that is
existing, pre-dated behavior (`caller.go:192-203`), not something this
feature introduces or worsens beyond making the dropped payload now include
memory content in addition to the rest of the system prompt (which is already
dropped on resume today, for every headless review call, memory or not).

Do not silently key `Pool.sessions` by `(FeatureKey, repoPath)` or add a
system-prompt-hash rotation trigger as part of this item — that is a
`Pool`-level correctness fix, candidate for its own follow-up backlog item,
and touches shared infrastructure used by every headless feature, not just
memory.

## Consequences

- Triage (the primary, most-frequently-invoked path) and the codebase-read
  review path get memory reliably on every call — no caveat.
- The plain-diff review path may review a diff using a stale operator-memory
  snapshot (from whenever the current `--resume` session last rotated) or,
  in a multi-repo backlog, a `REPO.md` snapshot belonging to a different
  repo than the one being reviewed, for up to 25 calls or until 3
  consecutive errors force rotation.
- This is a pre-existing gap in the Pool's session-reuse granularity that
  memory injection exposes rather than creates — the same drop already
  happens today to the non-memory portion of the system prompt on every
  resumed plain-diff review call.
- A follow-up item ("key headless Pool sessions by repo, or rotate on
  system-prompt-hash change") is the correct place to close this gap fully.
  Do not conflate that work with this item's storage/injection scope.
