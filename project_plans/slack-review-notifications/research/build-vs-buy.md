# Build vs Buy: Slack Outbound Notifications (Phase 1) + Inbound Interactive Callbacks (Phase 2)

**Date**: 2026-08-06
**Question**: Should stapler-squad hand-roll Slack integration with Go stdlib, or adopt an existing SDK/library, for (1) outbound incoming-webhook POSTs and (2) inbound interactive-callback signature verification?

## Baseline facts (verified)

- `go.mod` has **zero** Slack-related dependencies today (`grep -i slack go.mod go.sum` — no matches). This would be the repo's first Slack import of any kind.
- No existing `crypto/hmac` usage anywhere in the repo (`grep -rl "crypto/hmac" --include="*.go" .` — no matches) — Phase 2's signature verification has no prior art to extend in-repo.
- The repo already has a directly analogous precedent: `server/services/push_service.go` implements Web Push (VAPID) using stdlib crypto (`crypto/ecdsa`, `crypto/rand`, `crypto/sha256`) directly, and only pulls in a small third-party library (`github.com/SherClockHolmes/webpush-go`, already in `go.mod`) for the genuinely complex, easy-to-get-wrong part (AES128GCM push payload encryption). This establishes the repo's existing bar: **stdlib first, third-party only for the parts that are actually hard to get right.**
- Multiple existing services (`anthropic_client.go`, `domain_checker.go`, `session/backlog_plugin_github.go`, etc.) already build outbound HTTP calls directly against `net/http.Client` — no HTTP client wrapper library is used anywhere in the repo.

## Option 1: OSS library — `github.com/slack-go/slack`

Verified by downloading `v0.27.0` (latest, released 2026-06-27) via `go mod download` and reading source directly from `~/go/pkg/mod/github.com/slack-go/slack@v0.27.0`.

**Maturity/maintenance**: 60+ tagged releases from v0.0.1 through v0.27.0, actively released as of June 2026. License is BSD-2-Clause (`LICENSE` file, "Copyright (c) 2015, Norberto Lopes") — permissive, no conflict with this repo.

**Feature fit**:
- **Incoming webhooks**: `webhooks.go` exports `PostWebhook`/`PostWebhookContext`/`PostWebhookCustomHTTPContext`. Reading the actual implementation:
  ```go
  func PostWebhookCustomHTTPContext(ctx context.Context, url string, httpClient *http.Client, msg *WebhookMessage) error {
      raw, _ := json.Marshal(msg)
      req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
      req.Header.Set("Content-Type", "application/json")
      resp, err := httpClient.Do(req)
      // ...
  }
  ```
  This is exactly the ~20-line stdlib implementation the research question anticipated — the library adds a `WebhookMessage` struct and a status-code check, nothing more.
- **Interactive-callback signature verification**: `security.go`'s `NewSecretsVerifier` implements Slack's `v0=` HMAC-SHA256 scheme end to end — reads `X-Slack-Signature`/`X-Slack-Request-Timestamp` headers, does `hmac.New(sha256.New, secret)` over `v0:{timestamp}:{body}`, and compares with `hmac.Equal` (timing-safe). It also enforces a **5-minute replay window** (`if diff > 5*time.Minute { return ErrExpiredTimestamp }`) — a detail easy to omit when hand-rolling.
- **Block Kit**: Yes — `block*.go` (30+ files: sections, dividers, headers, images, rich text, tables, etc.) covers the full Block Kit spec, more than Phase 1's "session name, tool/summary, diff summary, link" needs.

**Dependency footprint**: The library's own `go.mod` requires `gorilla/websocket` (already in stapler-squad's `go.mod` v1.5.3, used for the web UI terminal) and `stretchr/testify` (already a dep, test-only). `go-test/deep` is used only in the library's own `_test.go` files — confirmed via `grep -l "go-test/deep" *.go | grep -v _test` returning no matches — so it never enters stapler-squad's build graph. **Net new dependency footprint: effectively zero** beyond the module itself (2.6MB on disk, 171 non-test `.go` files).

**What's overkill**: The package also ships RTM/websocket client, Socket Mode, full Web API bindings (channels, users, admin, OAuth, canvases, huddles, workflows...) — none of which Phase 1/2 need. Because Go compiles whole packages, importing `github.com/slack-go/slack` at all pulls the entire root package (including `rtm.go`, `websocket*.go`) into the build, even though only `webhooks.go` and `security.go` are used. This is the "full SDK for a 20-line POST" concern the research question raised, and it's a real cost — not of runtime dependency weight (negligible here), but of API surface and audit burden for a security-sensitive feature.

**Verdict: Viable, but not recommended for either phase given how thin the actual need is** (see Option 3 for why hand-rolling both pieces is the better call here specifically).

## Option 2: SaaS/managed relay (Zapier, IFTTT, generic webhook-to-Slack relay)

Slack's own **incoming webhooks are already a free, first-party feature** — no relay is needed to get a JSON POST into a Slack channel. A relay would only make sense if stapler-squad *couldn't* reach Slack directly, which isn't the case (Phase 1 is outbound-only from the self-hosted instance).

Checked Zapier's 2026 pricing as the representative option: the **Free plan (100 tasks/month) does not support webhooks at all** — webhook triggers/actions require the Professional tier ($73.50/mo annual) or higher [Zapier Pricing 2026, nocode.mba / zapierpricing.com]. Using Zapier here would mean: paying a recurring SaaS fee, adding a second external service as a single point of failure between the review queue and Slack, and adding latency/complexity — to replace a single `POST` call stapler-squad can make directly for free.

**Cost**: $0 (Slack direct) vs. $73.50+/mo (Zapier Professional, and Free tier doesn't even support webhooks).
**Complexity**: Slack direct = one config field (webhook URL) + one HTTP call. Zapier = external account, Zap configuration, a new failure domain, and (per this repo's `NotificationService` reliability requirement) another thing to make "failure must not block the review queue" against.
**Fit for a single self-hosted user**: None — this is infrastructure sized for connecting SaaS products that can't otherwise talk to each other. stapler-squad and Slack can already talk to each other directly.

**Verdict: Not recommended.** No scenario in this feature's scope benefits from an intermediary; it adds cost and a failure domain for zero functional gain over calling Slack's webhook URL directly.

## Option 3: LLM-generated implementation vs. battle-tested library, specifically for Phase 2 HMAC verification

This is the sharpest build-vs-buy tradeoff in the project, because Phase 2's signature verification is explicitly called out in the requirements as security-load-bearing: *"anything that can call `/api/hooks/permission-request`-equivalent effectively has agent-approval authority, so verifying the request actually came from Slack (signing secret) is a hard requirement, not optional hardening"* (requirements.md, Constraints).

Slack's algorithm (documented at Slack's own API docs, and mirrored exactly in `slack-go/slack`'s `security.go` above) is:
1. Concatenate `v0:{timestamp}:{raw_request_body}`.
2. HMAC-SHA256 it with the signing secret.
3. Hex-encode, prefix `v0=`, compare to the `X-Slack-Signature` header using a **timing-safe** comparison.
4. Reject if `X-Slack-Request-Timestamp` is more than 5 minutes old (replay protection).

Every step maps to a stdlib primitive: `crypto/hmac`, `crypto/sha256`, `encoding/hex`, `hmac.Equal` (already timing-safe by design — this isn't a subtle thing to get right, it's calling the right stdlib function instead of `bytes.Equal`), and a `time.Since` bounds check. The reference implementation in `slack-go/slack` is 40 lines including comments and error types — confirming the research question's "~15 lines" estimate was roughly right, and that there's no exotic cryptography here (no custom timing-safe comparison to implement, no novel replay-window logic).

The two failure modes this needs to avoid are well-known and easy to checklist:
- Using `bytes.Equal`/`==` instead of `hmac.Equal` for the comparison (timing side-channel).
- Omitting or getting the replay-window check wrong (forgetting it entirely, or comparing against wall-clock without a tolerance for clock skew).

Both are catchable in code review and covered directly by a unit test with a fixed secret/timestamp/signature fixture (Slack publishes example vectors) — this doesn't require importing a library to get right, but it does require **discipline to actually write that test**, which the project's existing NFR "Unit tests for the notifier" scope already commits to.

**Verdict: Safe to hand-roll**, provided:
- The implementation uses `hmac.Equal` (not manual byte comparison) — call this out explicitly in the plan/code review checklist.
- The 5-minute replay-window check is included from the start, not added later.
- A unit test locks in both behaviors (reject on bad signature, reject on stale timestamp, accept on valid) using a fixed test vector, so a future refactor can't silently regress either property.

This mirrors the repo's own `push_service.go` precedent: hand-roll the stdlib-shaped crypto (ECDSA key generation, HMAC), reach for a library only where the actual algorithm is nontrivial (VAPID's AES128GCM payload encryption in `webpush-go`'s case). Slack's HMAC signature scheme is the "stdlib-shaped" case, not the "nontrivial algorithm" case.

## Option 4: Fork or adapt existing code in this repo

Confirmed there is no existing Slack-related code, vendored or otherwise, anywhere in the repo (`go.mod`/`go.sum` grep, plus no `webhook`/`Webhook`/`Slack` references outside this project's own planning docs per requirements.md's baseline). This would be a first-time integration regardless of build-vs-buy choice. The nearest reusable pattern is architectural, not code: `push_service.go`'s "stdlib crypto + one narrowly-scoped third-party lib" shape, and the existing `net/http.Client`-direct pattern used by `anthropic_client.go` et al. for outbound calls.

## Summary table

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| `slack-go/slack` SDK | Battle-tested webhook POST + signature verification (incl. replay window, timing-safe compare) in ~2 files worth of actual use; BSD-2-Clause; near-zero net new dependency weight (shares `gorilla/websocket`/`testify` already in `go.mod`); Block Kit builders available if message formatting grows complex later | Whole-package import pulls in RTM/websocket/OAuth/admin-API code stapler-squad will never call, widening audit surface for a security-sensitive feature; adds a first-ever Slack dependency for what's individually two small, well-understood primitives | **Viable**, not the pick |
| SaaS relay (Zapier/IFTTT) | None specific to this use case — Slack incoming webhooks already work standalone | Recurring cost ($73.50+/mo for webhook support on Zapier); new external failure domain between review queue and notification; solves a "two SaaS apps can't talk" problem this feature doesn't have | **Not recommended** |
| Hand-rolled stdlib (both phases) | Zero new dependencies; ~20 lines for Phase 1's webhook POST (matches `slack-go/slack`'s own internal implementation almost line for line); ~40 lines for Phase 2's HMAC verification using only `crypto/hmac`/`crypto/sha256`/`hmac.Equal`; matches existing repo precedent (`push_service.go`); minimal surface to audit/maintain for a single-user, low-volume feature | Team owns correctness of the HMAC replay-window and timing-safe-compare details (though both are single well-documented stdlib calls, not novel crypto); no free Block Kit builder if message formatting needs grow — plain `map[string]interface{}`/struct-literal JSON is sufficient for Phase 1's scope (session name, tool/summary, diff snippet, link) | **Recommended** |
| Fork/adapt existing repo code | N/A | No existing Slack code to fork; not applicable | **Not applicable** |

## Recommendation

**Build with Go stdlib for both phases**, matching the repo's existing pattern (`push_service.go`) of stdlib-first with third-party libraries reserved for genuinely nontrivial algorithms:

- **Phase 1**: `net/http.Client` + `encoding/json` to POST a `WebhookMessage`-shaped struct (username/text/blocks) to the configured webhook URL — mirrors `slack-go/slack`'s own `PostWebhookCustomHTTPContext` almost exactly, and matches the outbound-HTTP pattern already used by `anthropic_client.go`/`domain_checker.go`.
- **Phase 2**: `crypto/hmac` + `crypto/sha256` + `hmac.Equal` implementing Slack's documented `v0:{timestamp}:{body}` scheme, with the 5-minute replay-window check and a fixed-vector unit test from day one — modeled directly on `slack-go/slack`'s `security.go` (read during this research, not blindly copied) as a correctness reference, not as a dependency.

Do not adopt `slack-go/slack` as a dependency: the two primitives needed are individually simple, well-documented, and already have a clear correctness reference to model against; importing the full SDK would trade a negligible dependency-weight cost for an unnecessarily wide audit surface on a feature whose Phase 2 half is explicitly security-sensitive. Revisit this decision only if a future phase needs deep Block Kit composition or other Web API calls (e.g. posting via a bot token instead of an incoming webhook) — at that point the SDK's Block Kit builders and broader API coverage would earn their weight.
