# Architecture Review: web-transport-architecture-review
**Date**: 2026-08-22
**Verdict**: CONCERNS

## Constitution Check

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repository. No
constitution to check the plan against — N/A, no violations to report.

## Re-verification of prior BLOCKER

**Prior BLOCKER (Epic 1.2 / Task 1.2.1a — flag-gated routing bypass): CONFIRMED FIXED.**

- `project_plans/web-transport-architecture-review/implementation/plan.md`'s "Epic 1.2:
  Dropped" section (lines 300–312) explicitly removes the original conditional
  `StreamingWSBridge` un-registration and states the registration "stays exactly as it is
  today, unconditionally, forever."
- Verified directly against the live code, not just the plan text:
  `server/server.go:419-427` registers `wsBridge.Handler("/api")` for both
  `watchSessionsPath` and `watchReviewQueuePath` with no surrounding `if` and no flag check
  of any kind — the registration block is identical to (and untouched from) what it was
  before this review cycle. `git status --short` and `git log -3 -- server/server.go` confirm
  `server/server.go` has zero uncommitted changes and no commit from this plan-repair pass —
  consistent with requirements.md's "no code ships in this pass" appetite.
- Also verified: `server/server.go`'s `StartRemote` (`grep -n "Protocols\|TLSNextProto\|SetUnencryptedHTTP2" server/server.go` → no matches) sets neither `Protocols` nor
  `TLSNextProto`, matching the plan/ADR-001's claim that real ALPN HTTP/2 on `:8444` is
  already automatic and requires no server change.

Remediation was fully applied: the routing bypass is gone, `server/server.go` routing is
untouched, and the plan's own dependency graph and ADR-001 both consistently describe the
corrected (no-server-change) design.

## Re-verification of prior CONCERNS

**Concern 1 (flag-resolver placement inside `connectrpc_websocket.go`): RESOLVED — now moot.**
The revised plan carries no server-side flag at all (`NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING`
is now the *only* flag, frontend-only — plan.md's Domain Glossary and Risk Control sections,
and ADR-001's Decision section, are explicit and consistent on this). With no
`useConnectRPCNativeStreaming()` resolver to place, there is no placement decision left to
get wrong, and Epic 1.1's sole remaining task (1.1.1a) touches only
`server/server_integration_test.go` — `connectrpc_websocket.go` is not referenced by any task
in the revised plan.

**Concern 2 (integration test not validating streaming through the full chain): PARTIALLY
RESOLVED — root cause closed, but a related gap remains, rescoped below.**
The original concern's proximate cause — uncertainty about whether `middleware.Compress`
correctly passes through streaming responses under the (now-dropped) `Server.Protocols`
change — is genuinely closed: `plan.md`'s Unresolved Questions section marks it resolved,
and this was independently re-verified by reading `server/middleware/gzip.go` directly:
`middleware.Compress` skips all `/api/*` requests unconditionally before any
gzip/zstd wrapping, and this behavior is untouched since no `Server.Protocols` change is made
anywhere in the revised plan.

However, a closely related gap persists in the revised plan, worth flagging as a fresh
Concern (see below): no task anywhere in the plan adds automated test coverage for the
actual mechanism the whole ADR leans on — `StreamingWSBridge`'s fallback-forwarding path.

## New architectural claim spot-check: the `https://` runtime guard

The plan's central new claim — gating native-transport selection on
`opt.baseUrl.startsWith("https://")` reliably distinguishes "served by the TLS remote-access
listener" from "served by the plain `:8543` listener" — **checks out as sound** for this
repo's actual code, not just as an assumption:

- `web-app/src/lib/config.ts:14-29`'s `getApiBaseUrl()` has exactly two sources: an explicit
  `NEXT_PUBLIC_API_URL` build-time override (checked first), or
  `window.location.origin + '/api'` in the browser. The fallback path is not spoofable by a
  page load's own JS — it reflects the actual scheme the browser used to fetch the page, so
  for the two listeners this repo documents (`Server.Start` on plain `:8543`,
  `Server.StartRemote` on TLS `:8444`) it is a genuine runtime signal, not a build-time guess.
- The override path was checked for real-world ambiguity: `grep -rn NEXT_PUBLIC_API_URL`
  across the repo shows its only current production usage is `make dev-stack`
  (`Makefile:397-407`), which always wires it to a plain `http://localhost:<port>/api` value
  (per its own comment and `web-app/src/lib/__tests__/config.test.ts`'s example). No existing
  build in this repo sets it to an `https://` value that doesn't correspond to the real TLS
  listener, so the guard's correctness isn't just theoretical — it's true of every build
  config that exists today.
- The plan's own Unresolved Questions section (plan.md:208-216) already discloses the one
  remaining theoretical gap — a future TLS-terminating reverse proxy in front of `:8543` would
  make `window.location.protocol` read `https:` while the Go process behind it still only
  speaks HTTP/1.1, silently defeating the guard — and correctly marks it non-blocking since no
  such deployment exists today (confirmed: CLAUDE.md documents only the two listeners this
  plan accounts for).

**Verdict: sound, not a BLOCKER.** No real gap found. See Nitpicks for one small
strengthening suggestion.

## Blockers

*(none)*

## Concerns

- [ ] **Epic 1.1 / `ws_stream_bridge.go`** (rescoped from the prior review's Concern 2) — No
  task in the revised plan adds automated test coverage for `StreamingWSBridge`'s
  fallback-forwarding path (`server/services/ws_stream_bridge.go:44-62`), which is the actual
  mechanism the entire ADR-001 decision depends on for correctness ("already served correctly
  ... through the existing, unconditional WSBridge registration"). Verified this has zero
  existing coverage today: `find server/services -iname "*ws_stream_bridge*"` returns only the
  implementation file, no `_test.go`. Task 1.1.1a (the only new automated test in the revised
  plan) exercises `/health` over a forced-HTTP/2 client and explicitly disclaims testing
  anything beyond stdlib protocol negotiation ("Label this test's doc comment explicitly as
  confirming existing stdlib protocol-negotiation behavior, not testing newly-built code");
  Task 1.3.1c is a frontend unit test that mocks `createConnectTransport` rather than hitting a
  real server. The only place an actual `WatchSessions`/`WatchReviewQueue` native-transport
  call gets exercised end-to-end is Epic 1.4's manual, operator-duration trial — there is no
  automated regression protection for this path going forward.
  **Recommendation**: add an integration test (Epic 1.1, or a new small epic) that starts a
  server via `StartRemote`, makes a real `WatchSessions`/`WatchReviewQueue` streaming call
  through a native ConnectRPC HTTP/2 client against the TLS listener, and asserts multiple
  incrementally-flushed messages arrive — this both closes the residual "full chain" gap the
  original review raised and gives `ws_stream_bridge.go` its first-ever automated test.

## Nitpicks

- The Unresolved Questions entry about a future TLS-terminating reverse proxy defeating the
  `https://` guard (plan.md:208-216) frames the risk as purely hypothetical ("no such
  deployment exists today"). Worth tightening slightly: `getApiBaseUrl()` already has a live,
  code-level override mechanism (`NEXT_PUBLIC_API_URL`) that *could* introduce this exact
  ambiguity in a future build config, even though every current usage (`make dev-stack`) only
  ever sets it to `http://`. A one-line addition noting this existing knob (not just a
  future-proxy scenario) would make the non-blocking judgment call more legible to a future
  maintainer who adds a new `NEXT_PUBLIC_API_URL`-setting build path. Non-blocking.
- Prior review's ESM module-mock hoisting nitpick (spying on `createConnectTransport` in
  `watch-ws-transport.test.ts`) is already explicitly carried into Task 1.3.1c's own
  description ("double-check ESM module-mock hoisting behavior per the architecture review's
  Nitpick before assuming the spy pattern works as described") — no further action needed,
  noting only that the plan text correctly incorporated this instead of dropping it.

## What checked out clean

- **BLOCKER remediation**: routing bypass fully removed, `server/server.go` verifiably
  untouched (zero uncommitted diff, no code change to the unconditional bridge registration).
- **Concern 1 remediation**: fully resolved by eliminating the server-side flag entirely —
  there's nothing left in `connectrpc_websocket.go`'s territory for this plan to touch.
- **Scope boundary (bidi terminal path / `session/streamhub`)**: no task modifies
  `session/streamhub/` or `connectrpc_websocket.go`'s terminal-frame handling; the borderline
  item from the prior review (the alternate flag-resolver placement) no longer exists to be
  borderline about.
- **Consistency across artifacts**: plan.md, ADR-001, and the live code all agree on the
  corrected design (no server change, unconditional bridge, frontend-only `https://`-gated
  guard) — cross-checked line references (`server/server.go:419-427`, `1378-1424`,
  `ws_stream_bridge.go:44-62`, `web-app/src/lib/config.ts:14-29`) all match the actual files.
- **Executability**: spot-checked Epic 1.3's file/line references against the current
  `useShells.ts`, `useReviewQueue.ts`, `useSessionService.ts` — import and call-site line
  numbers match closely enough to be directly actionable, and `createConnectTransport` is
  confirmed already imported in `watch-ws-transport.ts` (no new dependency needed).
