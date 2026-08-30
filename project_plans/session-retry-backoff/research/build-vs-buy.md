# Research: Build vs. Buy — session-retry-backoff

**Date**: 2026-08-06
**Question**: should the multi-attempt exponential-backoff `RetryPolicy` for
crashed/stalled/tmux-lost session restarts (see `../requirements.md`) be
hand-rolled, sourced from an existing Go library, or delegated to a hosted
retry/queue service?

## 1. Existing OSS library for Go exponential backoff

**Checked**: `go.mod` at repo root.

```
$ grep -niE "backoff|retry|cenkalti|avast|sethvargo" go.mod
89:	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
```

`cenkalti/backoff/v5` is already in the module graph, but **indirect** —
`go mod why` traces it to `telemetry` → OpenTelemetry's OTLP gRPC metric
exporter's internal retry package, not anything this repo imports directly:

```
$ go mod why github.com/cenkalti/backoff/v5
github.com/tstapler/stapler-squad/telemetry
go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc
go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc/internal/retry
github.com/cenkalti/backoff/v5
```

No code under `session/`, `server/`, or `config/` imports it (`grep -rln
"cenkalti/backoff" --include="*.go" .` returns nothing outside go.sum).
avast/retry-go, hashicorp/go-retryablehttp, and sethvargo/go-retry are not
dependencies at all.

**Architectural mismatch**: all three candidate libraries (cenkalti/backoff,
avast/retry-go, go-retryablehttp) are designed to wrap a **synchronous call**
— `Retry(func() error { ... })` — retrying it in a blocking loop until it
succeeds or the policy gives up. This feature has no such call to wrap: the
driver goroutine (`session/session_driver.go`) needs to *compute a delay*,
*schedule* a delayed restart without blocking the goroutine (so the session
stays reachable for manual "Retry now" / stop during the wait — Functional
Requirement 2), and persist an attempt counter across that wait. That's a
scheduling primitive, not a call-retry primitive. Forcing `cenkalti/backoff`
into this shape means using only its `NextBackOff()` delay-calculation method
and none of its `Retry()` orchestration — i.e., depending on the whole
library to get a two-line formula.

**The formula itself**: `min(initial * 2^attempt, max)` is the entire scope
per the requirements (`initial_delay_seconds`/`backoff:exponential`/
`max_delay_seconds`, "no need to build a strategy-plugin system for one
mode"). A hand-rolled version:

```go
func backoffDelay(attempt int, initial, max time.Duration) time.Duration {
	d := initial * time.Duration(1<<uint(attempt))
	if d > max || d <= 0 { // overflow guard: shifted duration can wrap negative
		return max
	}
	return d
}
```

~10 lines, fully covered by a table-driven test in one file.

**Pros (hand-rolled)**:
- No new dependency; `cenkalti/backoff` stays where it belongs (transitive,
  OTel-only) rather than becoming a direct project dependency for an
  unrelated purpose.
- Matches `.claude/rules/interface-pollution-checklist.md`'s Go idiom
  guidance — don't add an abstraction/dependency for what a few lines
  express directly.
- Full control over the exact formula this repo's requirements specify
  (`initial * 2^attempt`, capped), rather than adapting to a library's
  slightly different knobs (`cenkalti/backoff`'s `ExponentialBackOff` adds
  randomization/multiplier fields not requested here).

**Cons**: none material — the risk of a hand-rolled numeric formula this
small is addressed in §3.

**Verdict: Recommended.** Hand-roll the formula; do not add a direct
dependency on `cenkalti/backoff` or any retry-wrapping library for it.

## 2. SaaS/managed retry-queue service (Temporal, Inngest, etc.)

**Checked**: `go.mod` and `web-app/package.json` for `temporal`, `inngest`,
`bullmq`, `agenda` — no matches in either.

```
$ grep -niE "temporal|inngest|bullmq|agenda" go.mod web-app/package.json
(no output)
```

The project has no existing dependency on any hosted workflow/job-queue
service. Stapler Squad is a single-user, localhost-bound desktop-adjacent app
(`localhost:8543`, per `CLAUDE.md`) already reachable and stateful entirely
through local JSON/SQLite-free config and state files under
`~/.stapler-squad/`. Introducing Temporal or Inngest would mean either
self-hosting a workflow engine (a new persistent service + its own storage,
disproportionate to "delay a single goroutine's restart by up to
`max_delay_seconds`") or taking on an external network dependency for a
single-user local tool that currently has none. The requirements doc
explicitly rules out even a local SQLite-backed retry queue for this same
reason ("this app already persists session/instance state via its existing
config/state files; don't add a new datastore for a per-session attempt
counter and a handful of history entries") — a hosted service is a strictly
heavier version of the exact thing already ruled out.

**Pros**: durable retry scheduling that survives process restarts natively;
built-in dashboards/observability.

**Cons**: new external/self-hosted service dependency, operational overhead,
massive capability mismatch for "wait N seconds then restart a goroutine,"
no existing integration point in this codebase, contradicts the app's
local-first architecture.

**Verdict: Not recommended.** Confirmed no existing dependency of this kind;
scale and architecture don't fit a single-user localhost app.

## 3. LLM-generated bespoke implementation vs. library — correctness risk

The formula (`min(initial * 2^attempt, max)`) is a pure, deterministic,
single-input-domain function: `attempt` is a small bounded non-negative
integer (`max_attempts` from requirements is expected to be single digits),
`initial`/`max` are configuration-supplied durations. Risk surface is narrow
and enumerable:
- **Overflow**: `1<<attempt` for large `attempt` before capping — mitigated
  by capping `max_attempts` in config validation (a policy with
  `max_attempts: 50` is a config bug regardless of backoff implementation)
  and by the guard shown in §1 (`d <= 0` catches the wrapped-negative case).
- **Off-by-one on `attempt` indexing** (0-indexed vs. 1-indexed first retry)
  — exactly the kind of bug a table-driven test with explicit expected
  values per attempt (0→initial, 1→initial*2, 2→initial*4, ...) catches
  immediately, and such a test is trivial to write for a 10-line function.
- **Cap boundary** (`d == max` exactly) — one more test case.

None of these risks are reduced by using a library instead of hand-rolling —
a library still requires the same test coverage to confirm it's wired with
the right initial/multiplier/cap values, and it adds its own API-shape risk
(e.g., `cenkalti/backoff`'s jitter/multiplier defaults would need to be
suppressed to match "plain exponential, no jitter" if that's what's wanted).
A ~10-line function with 5-6 table-driven test cases has materially *lower*
integration risk than adapting a general-purpose retry library's
`Retry(func() error)` orchestration model to a "compute a delay, schedule
async, allow cancellation" use case it wasn't designed for.

**Verdict: Recommended.** Hand-roll with thorough table-driven tests; no
meaningful correctness advantage from a library here.

## 4. Fork or adapt in-repo code — `session/backlog_lifecycle.go` /
   `session/backlog_remediation.go`

**Checked**: `session/backlog_remediation.go` (the actual delay-computation
logic behind `Storage.RemediationDue`, referenced from
`backlog_lifecycle.go`'s `*WithBackoffGate` family).

Read the real implementation (`session/backlog_remediation.go:31-129`):

```go
var remediationBackoffSchedule = []time.Duration{
	30 * time.Minute,
	2 * time.Hour,
	8 * time.Hour,
	24 * time.Hour,
	72 * time.Hour,
}
var MaxRemediationAttempts = int32(len(remediationBackoffSchedule))

func nextRemediationAt(attemptNumber int32, now time.Time) *time.Time {
	idx := int(attemptNumber) - 1
	if idx < 0 || idx >= len(remediationBackoffSchedule) {
		return nil
	}
	t := now.Add(remediationBackoffSchedule[idx])
	return &t
}
```

This is **not an exponential-backoff formula** — it's a fixed, hand-tuned
lookup table (30m → 2h → 8h → 24h → 72h; roughly but not exactly
doubling/4x, chosen deliberately per the file's own comment "sized for
OOM-restart bursts," not derived from `initial * 2^n`). `evaluateRemediation`
additionally layers in DB-row state (`RemediationAttempts`,
`NextRemediationAt`, `GraceBootTime`) and a "restart grace" concept (an
extra free attempt if the whole server process restarted since the row was
last checked) that has no analog in the session-retry-backoff requirements.
The entire mechanism is wired through `Storage.RemediationDue`, which reads
and writes `BacklogStuckState` ent rows — ORM-backed persistence for a
*different* entity (backlog items' automation-step retries) than what this
feature tracks (a per-session attempt counter + retry history on the
session/instance itself).

**Reusability assessment**:
- The *arithmetic* (`nextRemediationAt`) is not reusable as-is — it's a
  fixed schedule table, not the `initial * 2^attempt, capped` formula this
  feature's requirements specify. Adapting it to be configurable exponential
  math would mean rewriting it into essentially the §1 formula anyway.
- The *gating pattern* (`evaluateRemediation`'s decision tree: parked vs.
  not-due vs. granted vs. granted-with-grace) is a reasonable *shape* to
  imitate conceptually (cap check → time check → grant), but its
  restart-grace behavior and ent-row persistence are specific to backlog
  automation's failure modes (this app can OOM-restart mid-remediation-burst)
  and are out of scope here per the requirements ("No SQLite-backed retry
  queue... don't add a new datastore for a per-session attempt counter").
- Requirements explicitly scope this as out-of-scope-to-touch: "No changes
  to the *backlog item*-level retry/backoff-gate mechanisms already in
  `session/backlog_lifecycle.go`... those retry backlog *automation steps*
  ..., a different concept from retrying a crashed *agent session process*."

**Pros (adapt)**: proven pattern already reviewed/tested in this codebase;
consistent naming/logging conventions (`*WithBackoffGate`) if mirrored.

**Cons**: wrong math (fixed table, not exponential formula), wrong
persistence layer (ent/DB rows vs. requirements' explicit "no new
datastore"), extra unrelated complexity (restart-grace) that doesn't apply
to session-process retries, and requirements explicitly forbid touching this
code path.

**Verdict: Not recommended as source material to fork/adapt** — the
concepts are only superficially similar (both are "backoff gates"), the
actual formulas and storage models diverge, and the requirements doc
explicitly excludes this code from the feature's scope. Treat it purely as
prior art for *naming/logging conventions* if useful, not as code to copy.

## Summary

| Option | Verdict |
|---|---|
| Hand-rolled exponential backoff formula (~10-15 lines) | **Recommended** |
| `cenkalti/backoff` / `avast/retry-go` / `go-retryablehttp` as a direct dependency | Not recommended |
| Hosted retry/queue SaaS (Temporal, Inngest, etc.) | Not recommended |
| Fork/adapt `session/backlog_remediation.go`'s schedule+gate logic | Not recommended (wrong formula, wrong storage layer, explicitly out of scope) |

Build from scratch: a small, pure, table-tested `backoffDelay(attempt int,
initial, max time.Duration) time.Duration` function plus an in-memory (or
existing config/state-file-backed, per requirements) attempt counter on the
session/instance itself.
