# ADR-031: `Registry` + `LiveInstance` Type Split, Superseding ADR-030's Call-Site Classification

**Status**: Accepted
**Date**: 2026-06-30
**Deciders**: Tyler Stapler
**Supersedes**: `decisions/ADR-030-lightweight-read-path-over-decoupled-activation.md` (not reverted — its rejection of pure "decoupled activation" still stands; this ADR replaces its *implementation* with one that also closes the gaps found afterward)

---

## Context

A third adversarial review pass found ADR-030's call-site classification (Group A: convert to read path; Group B: call `Stop()` after use) does not close the leak it was meant to close:

- `LoadInstances()` constructs the entire persisted session list per call; per-match `Stop()`-after-use leaves every sibling `Instance` constructed in the same call leaking its actor forever.
- `daemon/daemon.go`'s periodic full-registry reconstruction can independently construct a live `*Instance` for a session that already has one elsewhere — once each gets its own actor, that's two actors racing to own one tmux session, a correctness bug, not a resource leak.

Both failure modes trace to the same root cause: `*Instance` is one type serving two roles (disposable read projection; the one-and-only live actor-owning handle), and every fix attempted so far relied on getting call-site discipline right — exactly the failure mode this whole migration exists to eliminate (see `requirements.md`'s background on `stateMutex`'s false confidence).

## Decision

Split the roles into two types, with a smart constructor that makes duplicate live handles for one session structurally impossible to construct — not merely discouraged by convention:

- `session.InstanceData` (existing) stays the free-to-construct, no-lifecycle read-only value.
- `session.LiveInstance` (new) is the actor-owning handle, obtainable **only** via `Registry.Acquire(sessionID) (*LiveInstance, release func(), error)`. `Registry` deduplicates by construction: it holds `map[sessionID]*entry` behind a mutex scoped to map access only, and `Acquire` either returns an existing entry (refcount++) or constructs-and-spawns exactly once.
- Cleanup becomes reference counting (`release()`), not per-call-site `Stop()` placement — the registry decides when an actor's lifecycle ends, not each caller.

`Registry` is built once in `server/dependencies.go`'s existing staged DI construction (`BuildCoreDeps`/`BuildServiceDeps`/`BuildRuntimeDeps`) and injected into every consuming service as a constructor parameter — no package-level global, per `architecture-best-practices.md`'s dependency-inversion principle and this repo's own existing convention for exactly this kind of shared, stateful dependency.

## Consequences

### Positive
- The illegal state (two live actors for one session) is unrepresentable, not merely tested-for: there is no code path that produces a second live actor for an already-registered session, because there is no second constructor.
- Removes ADR-030's remaining discipline requirement entirely — new call sites added to this codebase in the future automatically inherit correct lifecycle behavior by using `Registry.Acquire`, rather than needing to remember a rule.
- Fits this codebase's existing DI convention exactly (staged builder functions, constructor-parameter injection) — no new architectural idiom introduced.
- `Registry`'s narrow interface (Interface Segregation) makes it fakeable in unit tests without a real tmux/git backend.

### Negative / Accepted tradeoffs
- Larger diff than ADR-030's approach: ~30+ call sites convert to `Registry.Acquire`/`release()` or `InstanceData`, and `Registry` itself needs a full design/implementation pass (map+mutex+refcount, wired through `server/dependencies.go`'s existing stages).
- Introduces one new (intentionally narrow) lock — the registry's map mutex. This is not a regression against this migration's goal: it protects map membership only, is held for microseconds (map read/write, refcount increment), and is orders of magnitude narrower in scope than the per-field `stateMutex` being eliminated.
- Requires deciding exactly where in `BuildCoreDeps`/`BuildServiceDeps`/`BuildRuntimeDeps` the registry is constructed and how its lifetime relates to `ReviewQueuePoller`'s existing live-instance list — scoped as new research/planning work.
