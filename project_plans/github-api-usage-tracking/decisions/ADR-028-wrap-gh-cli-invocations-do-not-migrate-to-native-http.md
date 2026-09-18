# ADR-028: Wrap the 7 `gh`-CLI Invocations for Attribution; Do Not Migrate Them to the Native HTTP Client

**Status**: Accepted
**Date**: 2026-08-10
**Project**: github-api-usage-tracking

## Context

`requirements.md`'s Open Questions leaves migrate-vs-wrap explicitly open: "Should the
`gh`-CLI-shelling-out call sites be migrated to the native `http.Client` instead of wrapped
separately? … The migrate-vs-wrap technical approach itself is still open …left for Phase 3
planning + adversarial review to decide." `research/architecture.md` §3 enumerated migration risks
(field-shape parity, auth divergence, error-surface change, timeout change, test-seam swap) without
reaching a verdict, and recommended migrating only `GetPRInfoCtx`; the user subsequently widened
instrumentation scope to all 7 real call sites (2026-08-10) without settling the technique.

The 7 in-scope `gh` invocations, all in `github/client.go` (verified by grep for
`safeexec.CommandContext(ctx, "gh"`):

| Line | Function | Command |
|---|---|---|
| 266 | `GetPRInfoCtx` | `gh pr view --json number,…,reviews,reviewDecision,statusCheckRollup` |
| 567 | `GetPRComments` | `gh pr view --json comments` |
| 611 | `GetPRDiff` | `gh pr diff` |
| 634 | `PostPRComment` | `gh pr comment` |
| 667 | `MergePR` | `gh <merge args>` |
| 689 | `ClosePR` | `gh pr close` |
| 709 | `CloneRepository` | `gh repo clone` |

(`IsForkRepo`, `client.go:535`, is the 8th; it has zero callers anywhere in the repo — production
or test — and is deleted, not instrumented, per `requirements.md` Scope.)

Three facts settle the decision:

1. **The quota numbers are token-global, so wrapping does not lose quota fidelity.** GitHub's
   `X-RateLimit-Remaining`/`-Limit`/`-Reset` describe the *token's* budget, not the client's. Any
   quota `gh` spends is already reflected in the headers of the very next native request observed
   by the new `usageTransport`. What wrapping loses is **per-request attribution granularity for
   `gh` calls only** — not the gauge, not the exhaustion detection, not the pause behaviour.
2. **Migrating `GetPRInfoCtx` would *increase* API consumption.** `client.go:265` requests
   `reviews,reviewDecision,statusCheckRollup` in one `gh pr view --json` call, which `gh` services
   with a single GraphQL query. The plain REST equivalent needs at least three calls —
   `GET /repos/{o}/{r}/pulls/{n}`, `GET …/pulls/{n}/reviews`, and
   `GET …/commits/{sha}/check-runs` — because REST `pulls/{number}` carries neither
   `reviewDecision` nor a check rollup. Tripling the request count of the single hottest
   poller-driven call site, inside a feature whose Success Metric is "zero rate-limit
   exhaustions," is self-defeating.
3. **Two of the seven have no native-HTTP equivalent at all.** `CloneRepository`
   (`client.go:709`) performs a git clone; `MergePR`/`ClosePR` are mutations whose `gh` form also
   carries branch-deletion and auto-merge semantics this repo depends on. "Migrate everything"
   is not even well-defined for these.

## Decision

**Instrument all 7 `gh` invocations by routing them through one shared wrapper in
`github/client.go` — `runGHCommand(ctx, callSite CallSite, args ...string)` — which records
exactly one `APIUsageEvent` with `Fidelity = FidelityInvocationApprox` per subprocess. Migrate
none of them to `ghHTTPClient` in this feature.**

The wrapper is the *only* place in `github/client.go` permitted to construct a
`safeexec.CommandContext(ctx, "gh", …)`, making the counting-site inventory and the
attribution-label inventory literally the same list — closing `research/pitfalls.md` §2c's
"missing even one of the 8 reproduces undercounting at the attribution layer."

Three consequences of the approximation are handled explicitly rather than left implicit:

- **One invocation is counted as one request, and that is labelled as an estimate.**
  `gh` may issue more than one HTTP request per invocation (pagination, its own auth preflight).
  Events carry `fidelity=invocation_approx`, the UI renders `gh` rows with an "≈" marker and a
  tooltip, and the panel never sums approximate and exact rows into a single unqualified total
  without the marker (`research/pitfalls.md` §1c).
- **Exactly one counting point per physical request.** The `gh` wrapper counts subprocesses;
  `usageTransport` counts native round-trips. No call site gets a manual counter in addition to
  either — the double-counting hazard in `research/pitfalls.md` §1b.
- **The residual is measured, not assumed away.** A periodic `GET /rate_limit` probe (which does
  not itself consume core quota) gives a token-global ground truth. `plan.md` Story 5.1.1
  reconciles `limit - remaining` against the sum of tracked quota-charged events per reset window
  and surfaces an explicit "N requests unaccounted" figure in the panel. That turns the
  approximation into a *measured* error bar, which is what `requirements.md`'s Success Metric
  ("trustworthy enough to attribute cause") actually requires.

## Consequences

- **Additive, low-risk diff.** No behaviour of `GetPRInfoCtx`, `MergePR`, `CloneRepository` etc.
  changes — same subprocess, same flags, same stdout parsing, same error text. This preserves
  `requirements.md`'s Risk Control claim ("no feature flag needed — this is additive"), which a
  migration would have invalidated.
- **`gh`-driven consumption is still visible to `DefaultRateLimiter`.** Because the limiter's
  quota snapshot is refreshed from every native response *and* from the `/rate_limit` probe, a
  `gh`-driven exhaustion is detected within one probe interval even though no `gh` response is
  parsed directly. The pause-on-exhaustion behaviour (finally wired by this feature) therefore
  covers `gh` usage in effect, not just in principle.
- **A token-divergence blind spot remains, and is made visible.** `gh` resolves its token from
  `gh auth status`/`~/.config/gh/hosts.yml`, independent of this repo's `getGHToken`
  (`github/http_client.go:36`: `GITHUB_TOKEN` → `GH_TOKEN` → keychain). If the two resolve to
  *different* tokens, the `gh` calls draw from a different quota than the gauge shows.
  `plan.md` Story 5.1.2 adds a startup identity check comparing `gh api user --jq .login` against
  the native `GetCurrentUserLogin` and logs a WARN plus a panel banner on mismatch — the gap is
  reported, not silently smoothed over.
- **Follow-up debt is named with a trigger, not left open-ended.** Revisit migration for a
  specific call site only when that call site's REST equivalent is a *single* request (i.e. not
  `GetPRInfoCtx`) **and** header-exact per-request attribution for it has been shown to matter by
  real reconciliation residual, not speculation. `GetPRComments` (`client.go:567`) is the most
  likely first candidate: `GET /repos/{o}/{r}/issues/{n}/comments` is a single REST call.
- **Test surface is unchanged.** No existing `gh`-stub test seam has to be swapped for an
  `httptest.Server`, which `research/architecture.md` §3 correctly flagged as a
  test-infrastructure change rather than a prod-code change.

## Alternatives Considered

- **Migrate all 7 to `ghHTTPClient`** (the "arguably more correct" option in `requirements.md`'s
  Rabbit Holes): rejected on the three facts above — it increases request volume for the hottest
  call site, is undefined for `CloneRepository`, and swaps five distinct error surfaces and one
  timeout model inside a Complexity-4 feature that is otherwise purely additive. The correctness
  argument for it (one accounting story) is largely satisfied anyway by the token-global quota
  snapshot plus the `/rate_limit` reconciliation probe.
- **Migrate `GetPRInfoCtx` only** (`research/architecture.md` §3's recommendation): rejected —
  it is the *worst* single candidate for migration (3 REST calls replacing 1 GraphQL call), and
  it would leave the feature with three instrumentation paths (native, migrated-native, `gh`
  wrapper) instead of two, for no attribution gain the reconciliation probe doesn't already
  provide.
- **Parse `gh`'s stderr/exit codes for rate-limit signals**: rejected — `gh`'s rate-limit output
  is unstructured, version-dependent, and only appears on failure, so it yields nothing on the
  99% success path where the counting actually happens. The `/rate_limit` probe gives strictly
  better information for less coupling.
- **Set `GH_TOKEN` in the subprocess environment and route `gh` through a local proxy** to observe
  its HTTP traffic: rejected — a man-in-the-middle proxy for a locally-installed CLI is far more
  machinery (TLS interception, proxy lifecycle, failure modes) than a personal-project visibility
  feature justifies, and it would make `gh` failures depend on the tracker being healthy.
