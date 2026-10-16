# Build vs. Buy: Jules REST API Client

Agent 6, SDD Phase 2 research for `google-jules-integration`. Scope: the Jules
API client wrapper only (auth, request/response, retry) — not the broader
session-creation/PR-import feature, which is covered elsewhere in this
research phase.

## 1. Existing OSS library / Go SDK

**No official Google Go SDK exists.** Google's own generated-client repo
(`googleapis/google-api-go-client`) does not include Jules — it's not part of
the stable/GA Google Cloud API surface, consistent with Jules being an alpha
product with its own docs site (`developers.google.com/jules/api`,
`jules.google/docs/api/reference/`) rather than the usual
`*.googleapis.com` discovery-doc pipeline.

**One community Go module exists**: [`github.com/yuyu1815/jules-api`](https://github.com/yuyu1815/jules-api)
(Go code under its `go/` subdirectory, `go get github.com/yuyu1815/jules-api/go@latest`).
Verified via `gh api repos/yuyu1815/jules-api`:

| Signal | Value |
|---|---|
| Stars / forks | 1 / 0 |
| License | MIT |
| Created | 2025-10-04 |
| Last push | 2025-10-08 |
| Open issues | 0 |
| Archived | false |

The repo's own README opens with: *"We apologize for mistakenly presenting
this library as official in earlier versions. This is an unofficial
library."* — the maintainer previously mislabeled it as Google's official
SDK and had to walk that back. Combined with single-maintainer authorship, 1
star, zero community engagement, and no commits in ~11 months against an
API Google has explicitly said may change, this is not a dependency to trust
for a "confidential credential" integration.

**Verdict: Not recommended.** Don't add `yuyu1815/jules-api` as a `go.mod`
dependency. It's fine as a *reference* for endpoint/payload shapes when
hand-writing the client (see §3), but not as code we import and trust with
API keys.

## 2. SaaS/managed API (Jules itself)

Jules *is* the SaaS being integrated, not a component with a SaaS
alternative — the build-vs-buy question here is really "adopt Google's
managed agent" vs. "don't integrate at all" (already covered by
requirements.md's Alternative (a), rejected). Relevant vendor-risk facts
gathered for this pipeline stage:

- **Cost model** (public Jules pricing page, `blog.google` announcement):
  bundled into Google's AI subscription tiers, not billed per-API-call —
  Free (15 tasks/day, 3 concurrent), Google AI Pro $19.99/mo (100 tasks/day,
  15 concurrent), Google AI Ultra $124.99/mo (300 tasks/day, 60 concurrent).
  No separate metered API pricing was found; the per-user API key inherits
  whatever plan that Google account is on.
- **Data residency**: confirmed by API docs — Jules runs each session on a
  Google-managed cloud VM, so source code leaves the local machine. This
  matches the concern already flagged in requirements.md §Non-functional
  Requirements; no new information changes that risk, it's just confirmed
  from the vendor side.
- **Vendor lock-in / alpha risk**: API is explicitly alpha
  (`https://jules.googleapis.com/v1alpha` base path) — Google states specs,
  keys, and definitions may change with no deprecation-notice guarantee at
  this stage. There is no migration path if Google changes the surface;
  the only mitigation is architectural isolation (single adapter package,
  per requirements.md's Constraints), not anything available to buy.

**Verdict: N/A as a build-vs-buy axis** — adopting Jules is a product
decision already made by requirements.md's scope, not a component
substitutable with a build. Flagging the residency/lock-in facts above for
the planning phase's risk register.

## 3. Hand-written client vs. OpenAPI/REST codegen

Checked for an existing OpenAPI-codegen pattern in this repo:

```
find . -iname "*openapi*.yaml" -o -iname "*openapi*.json" -o -iname "*swagger*.yaml"
grep -rn "oapi-codegen\|openapi-generator\|swagger-codegen" Makefile
```

Both came back empty — **this repo has no OpenAPI/codegen pattern for
external REST APIs anywhere**, only the protobuf/ConnectRPC generation
(`make proto-gen`) for stapler-squad's *own* internal API, which is a
different concern (internal contract, not a third-party client).

Every existing external-API client in this repo is hand-written `net/http`,
not generated:

- [`server/services/gemini_limits_client.go`](../../../server/services/gemini_limits_client.go) —
  hand-written client for the Gemini API: builds requests with
  `http.NewRequestWithContext`, injects auth via header or `?key=` query
  param depending on credential type, parses JSON response bodies manually,
  reads rate-limit headers off the response. No retry/backoff — a single
  attempt per call, errors surfacing as Go `error` values.
- [`server/services/anthropic_client.go`](../../../server/services/anthropic_client.go)
  and `anthropic_limits_client.go` — same hand-written `net/http` pattern
  for the Anthropic API.
- `github/client.go` shells out to the `gh` CLI (`safeexec.CommandContext`)
  rather than hitting the GitHub REST API directly — not applicable as a
  precedent here since Jules has no CLI equivalent to shell out to.

No shared/reusable retry-backoff library exists at repo scope either —
`session/tmux/ssh_runner_backoff.go`'s `sshBackoff` is unexported and
tmux-SSH-specific; `executor/circuit_breaker.go` is a general executor
wrapper, not an HTTP-client concern. Each existing external client hand-rolls
what little resilience it needs inline (the Gemini/Anthropic clients above
have none beyond `http.Client{Timeout: ...}`).

Jules' MVP surface (per requirements.md: create session, poll session
status, list activities — 3 endpoints, single `X-Goog-Api-Key` header auth,
no OAuth dance, likely no pagination for activities at this scale) is
comfortably within what the Gemini/Anthropic clients already demonstrate:
~150-250 lines of hand-written `net/http` code per client. Reaching for a
generic OpenAPI-codegen tool would be the first of its kind in this repo,
introduce a new build-time dependency and generated-code-`.gitignore`
convention (mirroring the ent/proto pattern this repo already treats
carefully — see root CLAUDE.md's ent-gen policy) for a 3-endpoint surface
that doesn't need it, and — worse — Google hasn't published an OpenAPI spec
for Jules to codegen from in the first place (only the prose REST docs), so
codegen would mean hand-authoring an OpenAPI document *before* generating a
client from it: strictly more work than writing the client directly.

**Verdict: Recommended — hand-write, following the Gemini/Anthropic client
pattern.** No codegen tooling needed or available to justify one.

## 4. Fork or adapt an existing client in this repo

`server/services/gemini_limits_client.go` is the closest structural analog:
same vendor family (Google), same simple-header/query-param API-key auth
model, same "resolve credential → build request → parse JSON → surface
typed result" shape, and it already plugs into this repo's
`CredentialChain`/`Credential` abstraction
([`server/services/credentials.go`](../../../server/services/credentials.go))
— which also answers one of requirements.md's Open Questions: yes, a
per-user secret-storage pattern already exists (`CredentialChain` with
pluggable `CredentialSource`s: env var, config file, OAuth-derived,
ADC-derived). A Jules client should resolve its API key through that same
chain (e.g. a new `provider: "jules"` credential source reading from
config, matching how `AnthropicAPIKey` is read in `ConfigFileCredentialSource`)
rather than inventing a new secrets mechanism — confirming requirements.md's
Constraint that credential storage must fit an existing pattern.

`github/client.go` is not a good template to copy: its `gh`-CLI-shell-out
approach doesn't apply (no `jules` CLI to shell out to), even though its
higher-level concerns (typed `PRInfo`-style result structs, `ErrNoPR`-style
sentinel errors, `singleflight` for auth-check caching) are useful patterns
to imitate conceptually, just not to fork wholesale.

**Verdict: Recommended — adapt `gemini_limits_client.go`'s shape** (request
construction, credential resolution via `CredentialChain`, JSON
decode-then-lock pattern for concurrent-safe caching) as the starting
skeleton for a new `JulesClient`, rather than designing the wrapper from a
blank file.

## Summary

| Option | Verdict |
|---|---|
| Adopt `yuyu1815/jules-api` community Go module | **Not recommended** — 1 star, single maintainer, stale ~11mo, previously mislabeled itself as official |
| Codegen from an OpenAPI spec | **Not recommended** — no spec exists to codegen from, no codegen precedent in this repo, only 3 endpoints |
| Hand-write a thin `net/http` client | **Recommended** — matches this repo's only two external-LLM-API-client precedents (Gemini, Anthropic) exactly |
| Adapt `gemini_limits_client.go` + `CredentialChain` as the starting skeleton | **Recommended** — same vendor family, same auth model, already-solved per-user credential storage |
| Fork `github/client.go`'s `gh`-CLI-shell-out approach | **Not applicable** — no `jules` CLI exists to shell out to |
