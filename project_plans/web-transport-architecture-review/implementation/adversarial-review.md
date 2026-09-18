# Adversarial Review: web-transport-architecture-review

**Date**: 2026-08-22
**Verdict**: CONCERNS

## Blockers

None. Both prior BLOCKERS are resolved and independently re-verified against real code, not
just the plan's prose:

- **h2c-to-zero-browsers blocker**: verified fixed. The revised plan/ADR drops
  `SetUnencryptedHTTP2` entirely and instead relies on `Server.StartRemote`'s already-shipped
  TLS listener (`server/server.go:1378-1424`). Read that function directly: `remoteSrv` sets
  `TLSConfig` and serves via `ServeTLS`, and sets neither `Protocols` nor `TLSNextProto`.
  `go doc net/http.Server` (run against this repo's installed `go1.26.4`, matching `go.mod`'s
  `go 1.26.3` requirement) confirms: *"If Protocols is nil, the default is usually HTTP/1 and
  HTTP/2... If TLSNextProto is non-nil and does not contain an 'h2' entry, the default is HTTP/1
  only."* Since both are nil here, HTTP/2-via-ALPN is on by default — this has been true since
  Go 1.6, not a 1.24+ feature, exactly as the plan claims. No `SetUnencryptedHTTP2`/h2c call
  exists anywhere in the codebase (`grep -rn SetUnencryptedHTTP2` returns nothing).
- **Unguarded server-wide protocol-change blocker**: verified fixed by removal, not mitigation.
  `server/server.go` is untouched by this plan (confirmed: `StartRemote` and the
  `wireDepsIntoServer` registration block, `server/server.go:419-427`, are unchanged from
  today's shipped behavior — `StreamingWSBridge` is registered unconditionally on both listeners
  exactly as before). Read `ws_stream_bridge.go:44-62` directly: `Handler()` already forwards
  every non-WebSocket-upgrade request straight to the wrapped Connect handler, so a
  `createConnectTransport` client is served correctly with zero routing change — the plan's
  "zero server code changes" claim holds. Since no `Protocols`/`TLSNextProto` change touches the
  plain `:8543` listener at all, the `http.Hijacker`-dependent WS-upgrade paths
  (`StreamTerminal`, VNC, CDP) sharing that listener's mux are provably unaffected by
  construction, not by argument.

## Concerns

- [ ] **The plan's "no such deployment exists today" claim (Unresolved Questions, second bullet)
  is inaccurate — a TLS-reverse-proxy-in-front-of-`:8543` pattern is already documented in this
  repo.** `.claude/docs/slack-phase2-public-reachability.md` shows a real, in-repo Caddy/nginx
  config (`listen 443 ssl; ... proxy_pass http://127.0.0.1:8543`) that TLS-terminates in front of
  the plain listener for the Slack-interactivity path. Today it explicitly forwards only
  `/api/hooks/slack-interactive` (`location / { return 404; }`), so it does not currently expose
  `WatchSessions`/`WatchReviewQueue` through `https://` framing while the backend still speaks
  HTTP/1.1 — the guard's failure mode isn't triggered *yet*. But the precedent for this exact
  topology already exists in the operator's own docs, one config-line away from the failure mode
  the plan's own Unresolved Question describes (a page served over `https://` while the Go
  process behind the proxy negotiates only HTTP/1.1, silently reintroducing the connection-slot
  regression the `https://` guard exists to prevent). **Recommendation**: correct the Unresolved
  Question to acknowledge this precedent rather than claim no such deployment exists, and
  consider a stronger runtime signal than the `baseUrl` protocol string alone (e.g., feature-detecting
  the actually-negotiated HTTP version from a response, or a one-line addition to
  `slack-phase2-public-reachability.md`'s "do not tunnel the whole port" guidance warning against
  ever forwarding `/api/session.v1.SessionService/Watch*` through a proxy that doesn't itself
  speak HTTP/2 to the backend). Non-blocking today because current exposure is zero, but the
  plan's confidence claim should match what's actually in the repo.

- [ ] **No automated browser/e2e coverage of the native-HTTP/2 streaming path exists or is added
  by this plan — and the plan doesn't say why.** The task list's only tests are: a Go integration
  test (Task 1.1.1a) that confirms stdlib ALPN negotiation with a non-browser client (correctly
  scoped now — it no longer claims to validate the browser-facing win, addressing the prior
  review's Minor), and a frontend unit test (Task 1.3.1c) that spies on which transport
  constructor is called, without exercising a real `fetch()` stream against a running server.
  Checked `tests/e2e/helpers/test-server.ts`: it has no TLS/remote-listener (`StartRemote`)
  support today, so a genuine Playwright e2e test of this exact path isn't currently possible
  without first extending the harness — a real infrastructure gap, not an oversight. The plan
  relies entirely on Story 1.4.1's manual, operator-run trial to catch a regression before the
  default flip. Given the change is dark-launched (off by default), provably inert on the primary
  `:8543` listener, and low-traffic (remote-access only), this is a reasonable interim position —
  but the plan should say so explicitly (harness gap → manual trial is the mitigation) rather than
  silently omitting automated coverage. **Recommendation**: add one sentence to Epic 1.4 or the
  Risk Control section naming the harness gap as the reason automated e2e isn't in scope, so a
  future reader doesn't mistake the omission for an oversight.

- [ ] **The revised plan's benefit is narrower than its own framing suggests, and this plan adds
  code rather than removing any.** Story 1.4.2 explicitly establishes `StreamingWSBridge` is
  never deleted — it is now a *permanent* dual-path system: `createWatchTransport` (WS-bridge)
  and `createConnectTransport` (native), selected by a guard function, forever, on both
  listeners. Verified against the actual file (`watch-ws-transport.ts`): the plan adds one new
  exported function plus its test suite; nothing is removed. The realized benefit is HTTP/2
  connection multiplexing plus retiring the *client-side* use of the hand-rolled envelope for two
  RPCs, but *only* for traffic through the less-traveled TLS remote-access listener (per project
  memory, primarily the mobile app connecting to `https://onyx.staplerhome.internal:8444`) — the
  majority of usage (the default `:8543` deployment) is completely unaffected and gets none of
  this. The ADR's Alternatives Considered table calls this "a real, already-available
  improvement" without noting it is a net *addition* of permanent branching complexity (new flag,
  new guard function, new test matrix) rather than a simplification. This isn't a reason to
  reject the plan — the implementation cost is genuinely small (~15 total minutes across three
  tasks per the plan's own estimates) and the risk is well-contained by the `https://` guard — but
  the plan should be honest that "adopt" here means "optionally enable a narrow, low-risk,
  code-additive improvement for one deployment path," not "simplify/replace the RPC transport."
  **Recommendation**: add one sentence to the plan's opening Feature line or ADR-001's
  Consequences section stating explicitly that this decision adds code and does not reduce
  `connectrpc_websocket.go`'s or `ws_stream_bridge.go`'s line count, so a reader sizing the
  follow-on session's effort against payoff has accurate expectations. This is not a
  recommendation to skip implementation and only document the finding — the plan as scoped is
  small enough and safe enough to be worth executing if/when the operator picks it up; it just
  shouldn't be sold as bigger than it is.

## Minors

- **CI Go-version pin count is slightly stale in the plan's own accounting, though its
  conclusion still holds.** Re-checked all workflow files: 7 are pinned to `go-version: '1.25.0'`
  (matching the plan's count), but 2 more (`demo-publish.yml`, `ux-analysis.yml`) are pinned to
  `'1.23'` — further behind `go.mod`'s `go 1.26.3` than the plan's "7 of 8" framing implies, and
  not mentioned at all. This doesn't change the plan's conclusion — re-verified via
  `go doc net/http.Server` that this revised plan uses no Go 1.24+ API (`Protocols`/
  `TLSNextProto` are read, never set), so the toolchain gap remains genuinely non-load-bearing for
  this specific plan — but the "7 of 8 checked" figure undercounts the actual drift if anyone
  later cites this plan as the audit of that debt.
- **Iggy Go SDK deprecation nuance — confirmed fixed.** `research/pitfalls.md` §1c now correctly
  distinguishes the archived standalone `iggy-rs/iggy-go-client` repo from the actively-developed
  `github.com/apache/iggy/foreign/go` monorepo SDK, addressing the prior review's Concern with
  the right level of nuance (flagged as "least battle-tested path," not "actively deprecated").
- **Unresolved Questions self-contradiction — confirmed fixed.** The `middleware.Compress`
  question is correctly marked resolved with the accurate citation
  (`server/middleware/gzip.go:160-163` skips all `/api/*` unconditionally, verified by direct
  read), and the real open question (WS-upgrade/HTTP-2 interaction) that the prior review said
  was missing is now moot on its own terms, since Epic 1.2's routing-bypass mechanism that would
  have created that interaction was dropped entirely.
