# Research: Stack — Slack Review Notifications

## Question
Which libraries/patterns apply to (a) Phase 1 outbound Slack incoming-webhook POSTs and (b) Phase 2 inbound Slack interactive-component signature verification, in this Go backend? Can stdlib alone do it, and what does this repo already do for comparable integrations?

## Recommendation: stdlib only, no new dependency

Both phases are fully covered by `net/http` + `encoding/json` (+ `crypto/hmac`/`crypto/sha256` for Phase 2). No Slack SDK should be added. This matches the repo's own established convention for every comparable outbound REST integration — see Existing Repo Conventions below. Do not add `github.com/slack-go/slack` (the most common community SDK) unless a future need (e.g. building complex Block Kit messages programmatically) makes hand-rolled JSON genuinely painful; even then, prefer adding just its `slackevents`/`slack.NewSecretsVerifier` sub-piece over the full client, since only the webhook POST + signature verification are needed.

## Existing repo conventions (`go.mod` at `/home/tstapler/Programming/stapler-squad/go.mod`)

- No HTTP client wrapper/framework is used anywhere. Every outbound REST call in the codebase constructs `http.NewRequestWithContext` + `json.Marshal`/`json.Unmarshal` by hand, even against SDK-rich APIs:
  - `github/http_client.go:17` — `var ghHTTPClient = &http.Client{Timeout: 30 * time.Second}` (package-level shared client)
  - `session/backlog_plugin_github.go:97,187,331,397` — raw `http.Client{Timeout: 30*time.Second}` per-call, `json.Marshal` bodies, manual status-code/rate-limit handling (see `CloseIssue`, lines ~308–334, for the canonical shape: build map payload → marshal → `http.NewRequestWithContext` → set headers → `Do` → check `resp.StatusCode`)
  - `server/services/anthropic_client.go:37`, `server/services/anthropic_limits_client.go:30`, `server/services/gemini_limits_client.go:37` — same `&http.Client{Timeout: N*time.Second}` pattern for other third-party APIs
  - `session/backlog_plugin_github_prs.go:97,177` — same pattern
- Notably, GitHub *does* have a well-maintained official-ish Go SDK (`google/go-github`) and this repo still doesn't use it — reinforcing that "roll it with stdlib" is the deliberate house style here, not an oversight.
- The one webhook-adjacent existing feature, `server/services/push_service.go` (Web Push notifications), does use a third-party library — `github.com/SherClockHolmes/webpush-go v1.4.0` (already in `go.mod`) — but that's because Web Push requires VAPID/ECDH crypto machinery stdlib doesn't provide out of the box. `sendToSubscription` (`server/services/push_service.go:225`) still follows the same shape: `json.Marshal` the notification body, then a single library call (`webpush.SendNotification`) that wraps the POST. Slack's incoming-webhook is a plain unauthenticated JSON POST with no such crypto requirement, so this precedent argues *for* stdlib, not for pulling in a Slack-specific analog of `webpush-go`.
- Secret handling: the repo already has an AES-256-GCM encryption-at-rest convention for exactly this kind of secret. `config.Config.GetOrCreateEncryptionKey()` (`config/config.go:988`) returns a 32-byte machine key; `session.EncryptToken`/`session.DecryptToken` (`session/backlog_crypto.go:14,39`) encrypt/decrypt values with it; `server/services/backlog_service_lifecycle.go:67-71` shows the call pattern (`cfg.GetOrCreateEncryptionKey()` → `session.EncryptToken(key, token)`). The Slack webhook URL and (Phase 2) signing secret should be stored the same way rather than as plaintext config fields — reuse `EncryptToken`/`DecryptToken`, don't invent a second secret-storage mechanism.
- The repo's own secret-redaction scanner already recognizes Slack tokens as a secret class: `session/backlog_review.go:33` — `{"slack_token", regexp.MustCompile(`xox[baprs]-[a-zA-Z0-9-]+`)}`. Confirms Slack secrets are a known category in this codebase's security tooling, even though no Slack integration exists yet. Note: incoming-webhook URLs (`https://hooks.slack.com/services/...`) and the Phase 2 signing secret don't match this `xox*` pattern (that pattern is for OAuth bot/user/app tokens), so this existing regex won't catch a leaked webhook URL or signing secret — worth flagging to whoever plans logging/redaction for this feature, not something to silently rely on for coverage.
- `go.sum` has zero Slack-related entries today (`grep -i slack go.sum` → empty) — confirmed no transitive Slack SDK is already vendored via another dependency.

## Phase 1 — Outbound incoming webhook (POST)

**Mechanism**: Slack Incoming Webhooks are a single POST of a JSON body to a per-workspace URL (`https://hooks.slack.com/services/T000/B000/XXXX`). No auth header — the URL itself is the secret. Response is `200 OK` with body `ok` on success, or a `4xx`/`5xx` with a plaintext error body (e.g. `invalid_payload`, `channel_not_found`) on failure.

**Payload shape** (community/Slack-documented current best practice):
- Prefer the **Block Kit** `blocks` array over the legacy top-level `text`/`attachments` fields for anything beyond a single line — this repo's requirements.md explicitly calls for structured content (session name, tool/summary, diff summary, dashboard link), which maps well to `section` blocks with `mrkdwn` text plus an `actions` block (Phase 2) for buttons.
- Minimal Go representation needs only `encoding/json` structs, e.g.:
  ```go
  type slackWebhookPayload struct {
      Text   string       `json:"text,omitempty"` // fallback/notification text
      Blocks []slackBlock `json:"blocks,omitempty"`
  }
  type slackBlock struct {
      Type string          `json:"type"`
      Text *slackBlockText `json:"text,omitempty"`
  }
  type slackBlockText struct {
      Type string `json:"type"` // "mrkdwn" or "plain_text"
      Text string `json:"text"`
  }
  ```
- **Size limits to respect** (per requirements.md's "Rabbit Holes" section, and Slack's documented limits): a single block's `text.text` is capped at 3000 characters; a message may contain at most 50 blocks; overall payload ~40KB. The diff-summary truncation this feature needs is therefore a hard requirement, not a nicety — truncate before building the block, don't rely on Slack to reject/truncate gracefully (a `4xx` on a payload that's too large is a *lost* notification, which directly violates this feature's "must not block/must be best-effort but should still land" intent).

**Delivery pattern to follow** (mirrors `session/backlog_plugin_github.go`'s `CloseIssue`):
```go
payload, err := json.Marshal(msg)
req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
req.Header.Set("Content-Type", "application/json")
resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
// treat any error / non-2xx as a logged, non-blocking failure per requirements.md's
// "Reliability" NFR — do not propagate as an error that fails the review-queue/approval flow
```

**Rate limiting**: Slack's informal limit for incoming webhooks is ~1 message/second per webhook URL, with bursts tolerated and `429`s returned with a `Retry-After` header when exceeded. Given this feature's Medium appetite and requirements.md's explicit note ("worth at least a de-dupe/coalescing note in planning, not necessarily a full backoff implementation"), stdlib's `golang.org/x/time/rate` — **already a direct dependency** (`golang.org/x/time v0.15.0` in `go.mod`) — is sufic for a simple limiter/coalescer if planning decides to implement one, with no new dependency needed either way.

## Phase 2 — Inbound interactive-component signature verification

**Mechanism**: Slack signs every interactive-component (and Events API) HTTP request with an HMAC-SHA256 computed from the app's **Signing Secret** (distinct from the webhook URL/OAuth tokens). Verification is a well-documented, stdlib-only recipe:

1. Read headers `X-Slack-Request-Timestamp` and `X-Slack-Signature` from the incoming request.
2. **Reject if the timestamp is more than 5 minutes old** (replay-attack protection) — compare against `time.Now().Unix()`.
3. Build the base string: `v0:{timestamp}:{raw_request_body}` (the *raw* body bytes, read before any JSON parsing — this is the most common implementation bug: parsing/re-marshaling the body before verifying changes byte-for-byte content like key ordering or whitespace and breaks the signature).
4. Compute `hmac.New(sha256.New, []byte(signingSecret))`, write the base string, hex-encode, prefix `v0=`.
5. Compare to the `X-Slack-Signature` header using `hmac.Equal` (constant-time — **never `==` or `strings.Compare`**, to avoid a timing side-channel on the secret).

All of this is `crypto/hmac`, `crypto/sha256`, `encoding/hex`, `net/http`, `strconv`, `time` — **entirely stdlib**, no dependency needed. Reference implementation shape:
```go
func verifySlackSignature(signingSecret string, header http.Header, rawBody []byte) error {
    ts := header.Get("X-Slack-Request-Timestamp")
    tsInt, err := strconv.ParseInt(ts, 10, 64)
    if err != nil {
        return fmt.Errorf("invalid timestamp: %w", err)
    }
    if abs(time.Now().Unix()-tsInt) > 5*60 {
        return errors.New("stale request (possible replay)")
    }
    base := "v0:" + ts + ":" + string(rawBody)
    mac := hmac.New(sha256.New, []byte(signingSecret))
    mac.Write([]byte(base))
    expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
    got := header.Get("X-Slack-Signature")
    if !hmac.Equal([]byte(expected), []byte(got)) {
        return errors.New("signature mismatch")
    }
    return nil
}
```

**Note on the community SDK's equivalent**: `slack-go/slack`'s `slackevents.NewSecretsVerifier` / `slack.NewSecretsVerifier` implements exactly this algorithm (and is a reasonable reference to diff against for correctness), but pulling in the dependency purely for ~20 lines of HMAC comparison that stdlib already covers doesn't fit this repo's demonstrated convention (see above) of not adding SDKs for well-documented single-purpose crypto/HTTP recipes.

**Payload format for interactive callbacks**: Slack POSTs interactive-component payloads as `application/x-www-form-urlencoded` with a single field `payload` containing URL-encoded JSON (not raw JSON body) — `r.ParseForm()` + `r.FormValue("payload")` + `json.Unmarshal`, all stdlib. This is a common integration gotcha worth flagging to the planning phase: the raw-body-for-signature-verification step (must happen before `ParseForm` consumes the body) and the urlencoded-wrapper-around-JSON parsing step are two distinct things that are easy to conflate.

**Reuse-vs-new-endpoint decision** (explicitly left open by requirements.md): translating a verified Slack button click into the existing `ApprovalHandler.HandlePermissionRequest` semantics is a routing/architecture decision for `sdd:3-plan`, not a stack question — no additional library considerations either way.

## Dependency verdict

| Need | stdlib sufficient? | Recommended package(s) |
|---|---|---|
| Phase 1: POST JSON to incoming webhook | Yes | `net/http`, `encoding/json`, `bytes` |
| Phase 1: rate-limit/coalesce bursts (if planned) | Yes | `golang.org/x/time/rate` (already a direct dep) |
| Phase 1/2: secret-at-rest storage | Yes | reuse `session.EncryptToken`/`DecryptToken` + `config.Config.GetOrCreateEncryptionKey()` |
| Phase 2: signature verification | Yes | `crypto/hmac`, `crypto/sha256`, `encoding/hex` |
| Phase 2: parse interactive payload | Yes | `net/http` (`r.ParseForm`), `encoding/json` |

**No new `go.mod` entries required for either phase.**
