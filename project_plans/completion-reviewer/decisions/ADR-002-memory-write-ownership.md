# ADR-002: This Item Owns the Memory Write Path, Built on Operator-Memory's Read/Scan Primitives

**Status**: Accepted
**Date**: 2026-08-22

## Context

`research/architecture.md` §3 sketches a `MemoryStore` interface with an `Append` method, to be
**injected at runtime** once #116 (operator-memory) lands a concrete implementation — mirroring
`SetSessionArchiver`/`SetReviewRespawner`-style package-level setters, with a nil-safe no-op until
then. This plan was asked to check that sketch against what operator-memory's own plan actually
commits to building.

Reading `project_plans/operator-memory/requirements.md` and
`project_plans/operator-memory/implementation/plan.md` directly surfaces a scope mismatch:

- `project_plans/operator-memory/requirements.md`, "Scope for this item": *"This issue creates the
  **storage + injection layer only — read-side**. It does not build the writer that populates the
  stores; a companion backlog item covers the background post-completion reviewer that writes to
  them automatically."*
- `project_plans/operator-memory/implementation/plan.md`'s Unresolved Question 1 confirms no write
  CLI/API ships in that item either: `opmemory.ScanForPromptInjection` is built and unit-tested but
  has **"no call site in this item — it exists ready for the companion writer to call."**

In other words: **operator-memory (#116) is explicitly naming *this* item as the writer.**
`completion-reviewer/requirements.md`'s "Hard Dependency" section, written before this
cross-check, describes #116 as providing "the actual memory store/write primitive" — that
framing is not what #116's own plan commits to. The real, verifiable dependency is narrower and
entirely stable already: three exported, fully-specified pure functions operator-memory's plan
commits to building in `session/opmemory`:

```go
// All three signatures are pinned in operator-memory/implementation/plan.md's Domain Glossary —
// stable enough to design against before that plan is implemented.
func OperatorMemoryPath() (string, error)
func RepoMemoryPath(repoPath string) (string, error)
func ScanForPromptInjection(content string) error
```

## Decision

Completion-reviewer **builds the write path itself**, as its own code, calling only the three
stable functions above — not as an injected `MemoryStore` implementation supplied by #116 at
runtime.

- `MemoryStore` (interface) and `MemoryEntry` (struct) are defined in `server/services/
  completion_reviewer.go` — the consumer package — per `.claude/rules/interface-pollution-
  checklist.md`. Unlike a typical single-implementation interface, this one is justified the same
  way `AIClient` already is in this codebase (`server/services/ai_interfaces.go`): one production
  implementation, fakeable for the orchestration unit tests the requirements explicitly ask for
  ("Unit tests cover: context assembly..., tool-restriction enforcement..., and the no-op path")
  — a testability seam, not a speculative extensibility one.
- `fileMemoryStore` (`server/services/completion_reviewer_memory.go`) is the one production
  implementation: it resolves `opmemory.RepoMemoryPath(entry.RepoPath)`, calls
  `opmemory.ScanForPromptInjection(entry.Content)` before writing (satisfying operator-memory's
  own requirement 6, "Content is scanned for prompt injection before any write"), and atomically
  appends a new Markdown section under a `github.com/gofrs/flock` lock — the same locking
  primitive `config/state.go` already uses for concurrent-writer safety on `~/.stapler-squad/`
  files (see Pattern Decisions in plan.md for why `flock`, not a bare `sync.Mutex`, is required
  here).
- Sequencing: this remains a hard **compile-time** dependency on `session/opmemory` landing first
  — `fileMemoryStore` directly imports `opmemory.RepoMemoryPath`/`ScanForPromptInjection`. This
  plan cannot merge before operator-memory's Epic 1 (path resolution) and Epic 2 (the scan
  function) land, even though it no longer depends on #116 shipping any write capability.
- Separately, `StartCompletionReviewerSubscriber` accepts `memoryStore MemoryStore` as an explicit
  parameter (nilable) rather than reading a package-level singleton — this is a **runtime**
  defensive gate (construction of `fileMemoryStore` could itself fail, e.g. home-directory
  resolution failing), independent of the compile-time dependency above, and mirrors
  `l.getSessionCreator() != nil` gating `spawnReviewGate` (`session/backlog_lifecycle.go:1010`).

## Alternatives Rejected

| Alternative | Rejected because |
|---|---|
| Nil-safe injected `MemoryStore`, concrete implementation deferred entirely to #116, `SetMemoryStore` setter called once #116 merges | Would leave completion-reviewer's AC "background reviewer only appends/updates memory" **permanently unimplemented** — #116's own plan explicitly declines to build this, so nothing would ever call the setter. This was architecture.md's original sketch, written without cross-checking operator-memory's actual committed scope; superseded here once that scope was read directly. |
| Add the write function into `session/opmemory` itself (extending #116's package with `opmemory.Append`/`AppendEntry`) | Makes this item's implementation depend on landing new code *inside* a sibling, independently-planned package rather than on that package's already-stable, already-fully-specified exported functions — re-couples two efforts the original requirements doc explicitly asked to keep independently developable ("a clean interface seam so the two efforts don't have to be co-developed by the same session"). Calling three stable, narrow, already-designed functions is a much thinner seam than contributing a new file to another plan's package. |

## Consequences

- Completion-reviewer's PR necessarily lands *after* operator-memory's Epic 1+2 merge (path
  resolution + scan function), not after the *entire* operator-memory item including its CLI
  (Epic 4) — a narrower, more precisely statable blocking dependency than "wait for #116."
- `session/opmemory` gets zero required changes from this item — it remains exactly the read-side
  package operator-memory's own plan describes.
- Completion-reviewer writes only to `REPO.md` (repo-scoped), never `OPERATOR.md` (fleet-scoped)
  — see plan.md's Pattern Decisions for the rationale (a single item's learnings are a claim about
  that repo, not a cross-repo generalization). Writing to `OPERATOR.md` is out of scope here and
  not designed against.
- AC "Pinned memory entries... bypass any auto-transition the reviewer might otherwise trigger" is
  **vacuously satisfied**: operator-memory's actual landed design (flat Markdown, no structured
  per-entry model) has no pinning concept for `fileMemoryStore` to check. Recorded as an explicit
  scope note in plan.md's Unresolved Questions rather than silently assumed or built against
  unspecified future structure.
