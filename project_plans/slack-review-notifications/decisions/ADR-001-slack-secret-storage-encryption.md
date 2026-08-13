# ADR-001: Encrypt Slack Webhook URL and Signing Secret at Rest

**Status**: Accepted
**Date**: 2026-08-06
**Deciders**: slack-review-notifications planning (sdd:3-plan)

## Context

`requirements.md`'s Non-functional Requirements section requires the Slack webhook URL and
(Phase 2) signing secret to "follow existing secret-handling conventions" without naming a
single mechanism. Research (`research/stack.md`, `research/architecture.md`,
`research/pitfalls.md`) found **two different existing precedents in this codebase for a
comparable value**, and no single one is "the" convention:

1. **`AnthropicAPIKey`** (`config/config.go:371`) — a plain `string` field on `config.Config`,
   persisted in plaintext in `config.json`, overridable via the `ANTHROPIC_API_KEY` env var at
   load time (`config/config.go:481,919`), with only a "do not log" comment as protection.
2. **Backlog `ItemSource` tokens** (`session/backlog_crypto.go`) — AES-256-GCM encrypted at
   rest via a 32-byte key from `config.Config.GetOrCreateEncryptionKey()`
   (`config/config.go:988`, itself persisted in `config.json` as `MachineEncryptionKey`),
   using `session.EncryptToken`/`session.DecryptToken`. Call pattern already established at
   `server/services/backlog_service_lifecycle.go:64-79` (`encryptAndMergeToken`): `key, _ :=
   cfg.GetOrCreateEncryptionKey()` → `session.EncryptToken(key, token)`.

No 1Password/vault runtime integration exists anywhere in the Go codebase (confirmed by grep
across `*.go` in `research/stack.md` and `research/features.md`) — that tool is
developer/bootstrap-time only (`bootstrap/roles/secrets`), not something the running server
process reaches for. Building a vault integration here has zero precedent and is out of this
feature's Medium appetite; ruled out without further discussion.

The remaining choice is genuinely between the two real precedents above, and the plan requires
picking one explicitly rather than silently defaulting to whichever is easier to code.

## Decision

**Encrypt both the Slack webhook URL and the Phase 2 signing secret at rest**, reusing the
existing `MachineEncryptionKey` + `session.EncryptToken`/`DecryptToken` primitive — the same
mechanism backlog `ItemSource` tokens already use, not a new one.

- `config.SlackConfig` (`config/types.go`) stores only ciphertext:
  `WebhookURLEncrypted string`, `SigningSecretEncrypted string`.
- Encryption/decryption happens in `server/services` (which already imports both `config` and
  `session` — see `backlog_service_lifecycle.go`), never inside the `config` package itself
  (`config` cannot import `session`: `session` already imports `config`, e.g.
  `session/instance.go:17` — importing it back would be a cycle).
- `SLACK_WEBHOOK_URL` / `SLACK_SIGNING_SECRET` env vars, when set, take precedence and are used
  as plaintext directly (never written to `config.json`) — mirrors `ANTHROPIC_API_KEY`'s
  env-override convention exactly, so the escape hatch developers already expect for secrets
  in this repo still works for Slack.

## Alternatives Considered

**Plaintext, like `AnthropicAPIKey`.**
Rejected. It's the simpler precedent to copy, but both Slack secrets are a worse fit for it
than an API key is: a Slack Incoming Webhook URL is a **bare bearer credential** — the entire
capability to post to the channel indefinitely is embedded in the URL string itself, with no
separate account/scope check the way an API key at least implies. The Phase 2 signing secret
is explicitly called out in `requirements.md`'s Constraints as gating **agent-approval
authority** ("anything that can call `/api/hooks/permission-request`-equivalent effectively has
agent-approval authority") — a materially higher blast radius than the Anthropic API key
(worst case: unexpected API spend) if `config.json` is ever exposed (accidentally committed,
screen-shared, pasted into a support ticket, backed up to a synced folder). The repo's own
`MachineEncryptionKey` doc comment already scopes it to "sensitive token data" generally, not
narrowly to `ItemSource` configs — extending its use to these two fields is a natural fit, not
a stretch.

**1Password / vault runtime integration.**
Rejected per `research/stack.md` and `research/features.md`: no such integration exists in Go
code anywhere in this repo; it would be net-new machinery invented for this feature alone, far
outside a Medium appetite, and the actual convention it would need to follow doesn't exist yet
to follow.

## Consequences

- **Save path** (`UpdateSlackConfig` RPC, Epic 1.4): must call
  `cfg.GetOrCreateEncryptionKey()` → `session.EncryptToken(...)` before writing
  `cfg.Slack.WebhookURLEncrypted`/`SigningSecretEncrypted`, then `config.SaveConfig(cfg)` — an
  empty request field means "leave the existing stored value unchanged," matching the
  mask-on-read UX (a user is never shown the real value to re-submit).
- **Read path** (`GetSlackConfig` RPC): must never decrypt-and-return the real value — only a
  `configured: bool` indicator (see `research/ux.md`'s masking guidance), consistent with how
  a bearer credential should behave in any admin UI.
- **Send path** (`SlackNotifier`, Epic 1.2): resolves the plaintext value via a small
  `resolveSlackWebhookURL(cfg)` / `resolveSlackSigningSecret(cfg)` helper in
  `server/services/slack_notifier.go` (env override first, else decrypt) from a **fresh
  `config.LoadConfig()` on every send**, matching the codebase's existing "fresh-load,
  last-write-wins" convention (`server/services/defaults_service.go`'s doc comment on
  `UpdateGlobalDefaults` names this pattern explicitly for the same reason: no cache to go
  stale after a config save). This also sidesteps needing a cache-invalidation/refresh
  mechanism when a user changes the webhook URL via the settings UI — the next send simply
  observes the new value. AES-GCM decrypt of a short string is microseconds; at a
  human-latency notification cadence (at most a few sends per minute), the cost of a fresh
  decrypt per send is not worth trading away correctness for.
- **Cost**: this is more code than copying `AnthropicAPIKey`'s plaintext field verbatim (one
  helper function per secret, plus encrypt-on-save/decrypt-on-use wiring) — but the primitive
  itself (`EncryptToken`/`DecryptToken`) already exists and is already exercised in production
  by the backlog-token path, so no new cryptography is written for this feature, only new call
  sites of an existing one.
- **Follow-up**: if a future feature needs the exact same "encrypted secret field on
  `config.Config`" shape a third time, it's worth factoring `resolveSlackWebhookURL`-style
  helpers into a small shared `config`-adjacent package rather than a third bespoke
  implementation — not done here since two instances (backlog tokens, Slack) don't yet justify
  the abstraction (see `.claude/rules/interface-pollution-checklist.md`'s "don't generalize
  from a single call site" spirit, applied to a helper function rather than an interface).
