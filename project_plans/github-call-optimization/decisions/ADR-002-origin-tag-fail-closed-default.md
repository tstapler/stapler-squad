# ADR-002: An untagged GitHub-call context defaults to the background (poller) tier, not interactive

**Status**: Accepted
**Date**: 2026-09-08
**Project**: github-call-optimization

## Context

This project introduces `github.CallOrigin`, threaded via a `context.Context` value, following
the exact shape of the existing `prFixTriggerSourceKey`/`withPRFixTriggerSource`/
`prFixTriggerSourceFrom` precedent (`session/backlog_lifecycle_pr.go:1662-1685`). That precedent's
`prFixTriggerSourceFrom` defaults an untagged context to `"poller"` — but it's read at exactly one
call site, a log line (`session/backlog_lifecycle_pr.go:1274`). Mislabeling a log line is cheap.

This project's new `GitHubCallOriginFrom(ctx)` is read inside `rateLimitTransport.RoundTrip` (via
`AdmitOrigin`, Phase 3) to make an actual admission-control *decision* — a materially higher-stakes
read (`research/architecture.md` §3's explicit caveat). If the default value is `OriginInteractive`
(the tier that's never rejected by `AdmitOrigin`), then any background-origin call site that
forgets to tag its context — a bug, not a deliberate choice — silently and invisibly escapes
admission control entirely, rather than failing loudly or even just mislabeling a metric.

## Decision

`GitHubCallOriginFrom(ctx)` defaults an untagged context to `OriginPRStatusPoller` — a background
tier subject to `AdmitOrigin`'s reserved-headroom rejection — not `OriginInteractive`. This is the
opposite default direction from `prFixTriggerSourceFrom`'s existing precedent.

## Consequences

- **Fail-closed for admission control**: a background call site that forgets to tag its context
  gets *more* restrictive treatment (subject to headroom rejection) rather than escaping
  admission control. This matches the stated goal — protecting interactive traffic from
  background-caused exhaustion — even in the presence of a plumbing bug.
- **Trade-off, explicitly accepted**: if an *interactive* call site (one of the 5 RPC handlers in
  `server/services/github_service.go`) forgets to tag its context, it would incorrectly receive
  background-tier treatment instead of being exempted — the failure mode is now "an interactive
  action might back off when it shouldn't," rather than "a background action escapes throttling
  entirely." Given there are only 5 interactive call sites (enumerable and covered by Phase 1's
  Task 1.1.3d) versus a much larger and more distributed set of current-and-future background call
  sites, biasing the default toward protecting the many-callers case is the better trade.
- **Detectability**: Phase 1's `github.calls_total{origin=...}` metric makes a missed interactive
  tagging bug visible (elevated `origin="poller"` volume attributable to what should be interactive
  traffic) — this is a real, if imperfect, safety net; a `go vet`-style static check that every RPC
  handler tags its context was considered but not built in this project (flagged as a candidate for
  `quality:reflect-and-fix`'s enforcement-ladder thinking once/if a real instance of this bug
  surfaces, per `research/architecture.md` §3's suggestion).

## Alternatives Considered

- **Default to `OriginInteractive`, matching `prFixTriggerSourceFrom`'s existing precedent** —
  rejected: this is the exact risk this ADR exists to close (see Context above).
- **Require every context to be explicitly tagged, panic/error on an untagged read** — rejected as
  disproportionate for a single-user tool; would turn a missed-tagging bug into a hard crash on
  the admission-control path itself, which is worse than either graceful-degradation option.
