# Spike S6: remote-listener write-path review for Tier B actions

**Date**: 2026-10-18 | **Worktree**: `nam-spikeB` at `497016cde` | **Verdict: PASS for the chain; FAIL-fallback posture kept for Tier B (ADR-004 default stands). Two corrections to ADR-004's text, see "Findings".**

`InvokeNotificationAction` does not exist yet, so the review covers the route family it will live in: Connect RPCs on the shared `SessionService` mux under `/api/session.v1.SessionService/*`, exercised with the sibling write handlers `ReplyToPendingQuestion` and `WriteToSession`.

Probe target: a second isolated instance built from this worktree (`PORT=62873 --remote-access --remote-port 62874`, `STAPLER_SQUAD_INSTANCE=nam-spikeB`, tmux socket `ssq-manual-2`; `:62871/:62872` were held by a stale manual instance from 15:06, so the next block was used). The live `:8543` service was not touched.

## 1. Chain walk (code)

| Layer | Source | Effect on `:8444` |
|---|---|---|
| TLS + otelhttp | `Server.StartRemote`, `server/server.go:1990` | HTTPS only |
| `stampRequest(ListenerRemote, requiresAuth)` | `Server.stampRequest`, `server/server.go` | records listener + `AuthModeRequired` for in-handler verdicts |
| `Logging`, `CORSWithOrigins`, `Compress` | `Server.remoteChain`, `server/server.go` | no auth effect |
| **`middleware.Auth(sessions, WithSelfAuthenticatedPaths)`** | `main.go:1633`, `server/middleware/auth.go:60` | cookie `cs_auth` or `Authorization: Bearer`; unauthenticated `/api/*` -> **401 JSON**; any other path -> **302 /login** |
| `ProbeGuard` / `HostGuard` | local chain only (`Server.hostGuardConfig`/`probeGuardConfig` are wired on the local chain only; `remoteChain` comment: "never carries ProbeGuard") | **absent on `:8444`** |
| Exempt (no auth) | `auth.go` `exemptPrefixes` (`/auth/`, `/login`, `/health`, `/_next/`, `/favicon`), `exemptPeerPaths` (3 `/internal/*` gossip paths, handler-authenticated), `POST /webhooks/<slug>` single-segment slugs except `github` | none of these is an action handler |
| In-handler `EvaluateLocalWrite` | `server/services/local_write_verdict.go:57` | with `AuthRequired=true`: `RebindingVerdict` skips the Host test (keeps Origin test) and `LocalCallerVerdict` returns allowed (`callerguard.go:104-136`; pinned by `TestRebindingVerdict_should_SkipHostTestButKeepOriginTest_WhenAuthRequired` and `TestLocalCallerVerdict_should_RefuseRemotePeerOrProxiedLoopbackUnlessAuthRequired`, both PASS) |

State-changing GETs on the shared mux: the `GET` handlers are `/auth/credentials`, `/auth/local-login`, `/auth/status`, `/api/changelog`, `/api/update-check`, `/api/analytics/summary`, `/api/deep-link/resolve`, `/api/debug/log-level[/packages]`, webhook-management `capability`/`registrations/{id}` and the signed `/internal/claim-lookup` (`git grep -n 'HandleFunc("GET '`). The only `/auth/*` GET that mints a session is `/auth/local-login` (exempt path), which per `server/auth/localtoken.go` is a code-exchange handler (`POST /auth/local-login/code` mints the code; exchange semantics not exercised here). No Connect RPC is marked `NO_SIDE_EFFECTS` (the single proto hit is a comment saying it is deliberately not), so Connect GET is not enabled for any write RPC.

## 2. Live attempts against `:62874` (no credentials)

```
POST /api/session.v1.SessionService/InvokeNotificationAction       own=401 foreignHost=401
POST /api/session.v1.SessionService/ReplyToPendingQuestion         own=401 foreignHost=401
POST /api/session.v1.SessionService/WriteToSession                 own=401 foreignHost=401
GET  /api/session.v1.SessionService/ListSessions?connect=v1&encoding=json&message=%7B%7D   own=401 foreignHost=401
POST /session.v1.SessionService/ListSessions   (non-/api path)     own=302 foreignHost=302   (-> /login)
POST /mcp                                                           own=302 foreignHost=302
POST /api/hooks/permission-request                                  own=401 foreignHost=401
POST /webhooks/github                                               own=302 foreignHost=302
GET  /health                                                        own=200 foreignHost=200   (exempt, read-only)
bogus bearer  -> WriteToSession 401 ; bogus cs_auth cookie -> 401
local listener :62873 with Host: evil.example.com -> 403 (HostGuard works there)
```
("foreignHost" = `curl -H 'Host: evil.example.com'`.) The unknown RPC `InvokeNotificationAction` returns the same 401 as real ones because auth runs before routing, so an unauthenticated caller reaches no handler.

## 3. Findings

1. **Unauthenticated writes cannot reach any handler on `:8444`** (401 for `/api/*`, 302 otherwise). This satisfies the S6 PASS condition's authenticated-chain requirement and the "401/403 and no handler runs" Given/When/Then.
2. **The AC wording "with the Host allowlist" does not hold for `:8444`**: there is no Host allowlist on the remote chain; `RebindingVerdict` deliberately skips the Host test when auth is required and relies on the `cs_auth` cookie plus the Origin test. A foreign `Host` gets the same 401 as a correct one. Rebinding is blocked by the cookie (host-scoped, `SameSite=Strict`) and the Origin test, not by a Host list. Docs and ADR text must not claim a Host allowlist on `:8444`.
3. **ADR-004 context is inaccurate**: it says Reply is localhost-only today via `EvaluateLocalWrite`. The code allows an authenticated remote caller through `EvaluateLocalWrite` (`LocalCallerVerdict` returns allowed when `AuthRequired`), so `ReplyToPendingQuestion` and the steer handlers are reachable on `:8444` for a logged-in session today (not exercised with a real passkey session in this spike: UNVERIFIED end to end, verified at the verdict-function level by the two unit tests above). Consequence for the plan: **`InvokeNotificationAction` must enforce the Tier B `desktop_only` refusal itself** by reading the stamped listener (`RequestRecordFrom(ctx).Listener == ListenerRemote`), not rely on `EvaluateLocalWrite` to refuse remote callers.
4. Other exempt-path observation (not an S6 blocker, report only): `isGenericWebhookDelivery` exempts `POST /webhooks/<single-segment>` but the registered generic route is `POST /webhooks/generic/{slug}` (two segments), so on `:8444` it gets 302 `/login` (verified: `POST /webhooks/generic/x` -> 302). Either the exemption is stale or generic webhooks are meant to be local-only; not investigated.

## 4. Decision

- **PASS** (chain review): action handlers are reachable on `:8444` only through the authenticated chain; no state-changing GET; foreign Host does not bypass auth.
- **Tier B promotion is still not enabled by this spike**: ADR-004's default stands (Tier B `available=false, reason="desktop_only"` on the remote listener) until Story 2.8's Go integration test through the real remote chain (`server/remote_listener_actions_integration_test.go`) passes with an authenticated session. That test was **not written in this milestone**; it belongs to Story 2.8 and should assert: unauthenticated -> 401, authenticated Tier A -> allowed, authenticated Tier B live_keystroke -> `FORBIDDEN`/`desktop_only` from the handler's own listener check (finding 3).
- Cleanup: instance stopped and `tmux -L ssq-manual-2 kill-server` run at the end of the milestone.
