# Stack Research: PR Event Webhooks (CI Failure / Review Comments)

Scope: technology stack for extending the existing GitHub webhook receiver to
`check_run`/`workflow_run`/`pull_request_review`/`issue_comment` events and routing
them to `PRFixSpawner`.

## Sibling project reuse (project_plans/webhook-triggers/)

The sibling `webhook-triggers` project already built and shipped the infrastructure
this feature needs to extend, not build fresh:

- **HMAC verification**: `server/services/webhook_signature.go:20`,
  `VerifyGitHubSignature(secret, body, sigHeader)` — stdlib `crypto/hmac` +
  `crypto/sha256`, `hmac.Equal` for constant-time comparison. No third-party
  dependency; that research finding still holds and applies directly here (same
  `X-Hub-Signature-256` header, same verification call).
- **Shared webhook plumbing**: `server/services/webhook_trigger_common.go` —
  `readAndDecodeWebhookBody` (body read + JSON decode + dedup-on-error path),
  `decryptWorkflowSecret`, `persistTriggerFireEvent`, `claimAndFireTrigger`
  (dedup-and-fire against `TriggerFireEventRepository`, keyed by
  `X-GitHub-Delivery`). These are already generic over event payload shape
  (`map[string]interface{}`) and are directly reusable for the new event types —
  no new dedup/audit mechanism needed.
- **Route registration pattern**: `GitHubWebhookHandler.RegisterRoutes` registers
  `POST /webhooks/github` on `*http.ServeMux`
  (`server/services/github_webhook_handler.go:30`), wired in `server/server.go`
  behind a route-registration-time `webhook_triggers` flag check
  (`server/server.go:763`) plus a second defense-in-depth check inside
  `Handle` itself (`github_webhook_handler.go:40`). New event types stay on the
  same route/handler — GitHub multiplexes event type via the `X-GitHub-Event`
  header on a single endpoint, not separate URLs.

**Confirmed by reading `server/services/github_webhook_handler.go` in full (150
lines)**: `Handle` currently has no `X-GitHub-Event` header check at all — it
unconditionally treats every delivery as a push event and calls
`extractGitHubRepoAndBranch`, which only reads `payload["repository"]["full_name"]`
and `payload["ref"]`. Every event type this feature needs to add
(`check_run`/`workflow_run`/`pull_request_review`/`issue_comment`) does carry
`repository.full_name`, but none carries `ref` at the top level the way push
events do — so extending event-type support requires branching on
`r.Header.Get("X-GitHub-Event")` before dispatch, not just adding fields to the
existing payload extraction.

## Does this repo depend on google/go-github or any GitHub API client library?

**No.** `grep -n "go-github" go.mod go.sum` — zero matches. `grep -rl
"google/go-github" --include="*.go" .` — zero matches anywhere in the tree.

All existing GitHub REST API interaction (`session/backlog_plugin_github_prs.go`)
is hand-rolled: plain `net/http` requests against `githubAPIURL(...)`, decoded
into locally-defined structs (e.g. `githubCheckRun` used by `fetchCILabel` at
`session/backlog_plugin_github_prs.go:166-197`, which calls the check-runs REST
endpoint, not a webhook). This is a **different code path** (`PRStatusPoller`'s
REST polling) from the webhook receiver being extended here — it's useful
precedent for how this repo already models GitHub JSON shapes (small anonymous
or locally-scoped structs, not a full SDK), but it doesn't parse *webhook*
payloads today.

`grep -rln "check_run\|workflow_run\|pull_request_review\|issue_comment"
--include="*.go" .` matches only `pkg/classifier/classifier_test.go`,
`session/backlog_plugin_github_prs.go` (the REST poller above), and
`session/backlog_plugin_github_test.go` — no webhook-event-type handling exists
anywhere yet.

**Recommendation: do not add `google/go-github`.** The existing webhook handler
already decodes the body as `map[string]interface{}` (`readAndDecodeWebhookBody`)
and reads a handful of fields by key
(`extractGitHubRepoAndBranch`-style). The new event types need only a few
fields each:
- `check_run`: `action` ("completed"), `check_run.conclusion`
  ("failure"/"timed_out"/etc.), `check_run.check_suite.pull_requests[].number`
  (may be empty on forks — GitHub's documented limitation), `repository.full_name`.
- `workflow_run`: `action`, `workflow_run.conclusion`,
  `workflow_run.pull_requests[].number`, `repository.full_name`.
- `pull_request_review`: `action` ("submitted"), `review.state`
  ("changes_requested"), `pull_request.number`, `repository.full_name`.
- `issue_comment`: `action` ("created"), `issue.pull_request` (presence
  distinguishes a PR comment from a plain issue comment — GitHub sends
  `issue_comment` for both), `repository.full_name`.

Pulling in a full SDK (`go-github` alone is a large dependency: typed structs
for the entire REST/webhook surface, plus its own HTTP client wrapper) to read
4-6 known field paths out of an already-decoded `map[string]interface{}` is
the same "don't reach for a library until the plain version is proven
insufficient" call the sibling project made for HMAC and templating. A
small locally-scoped extractor function per event type (mirroring
`extractGitHubRepoAndBranch`'s style) is more consistent with this codebase and
avoids taking on `go-github`'s versioning/compat surface (see below) for a
handful of field reads.

If future work needs the *outbound* GitHub API (posting comments, re-running
checks, etc. — not in this requirements doc's scope), that would be the point
to reconsider `go-github` for the client side; it still wouldn't be needed for
inbound webhook payload parsing.

## Feature flag pattern — trivial to extend or gate under `webhook_triggers`

`config.Config.GetFeatureFlag` (`config/config.go:1329`) is a simple
`map[string]bool` lookup with no schema/enum to extend:

```go
func (c *Config) GetFeatureFlag(name string) bool {
	if c == nil || c.FeatureFlags == nil {
		return false
	}
	return c.FeatureFlags[name]
}
```

Any string key works with zero code changes beyond calling
`cfg.GetFeatureFlag("new_flag_name")` at the check site(s) and
`cfg.SetFeatureFlag("new_flag_name", true)` wherever flags get toggled (e.g. a
settings UI or seed config). The doc comment at `config/config.go:1326-1328`
lists recognized flags for humans only (currently just `"backlog"` — note
`"webhook_triggers"` itself isn't listed there either, so that comment is
already somewhat stale and not a source of truth/validation).

**Two viable options, both trivial:**
1. **Reuse `webhook_triggers`** — the new event types ride the same route,
   same handler struct, same flag gate already wired at both
   `server/server.go:763` (route registration) and `Handle`'s first line
   (`github_webhook_handler.go:40`). Simplest; matches requirements.md Goal 5's
   phrasing loosely ("gate new event-type handling behind a feature flag" — a
   shared flag still gates it, just coarser-grained).
   - Note: `session/chain_firer.go:145` and `:336` and
     `server/services/callback_dispatcher.go:120` all also gate on
     `webhook_triggers` today — it's already a fairly broad "is the webhook
     subsystem on at all" switch, not narrowly scoped to push-triggered session
     creation. Reusing it for PR-event reactions is consistent with its
     existing scope, not a stretch.
2. **New flag** (e.g. `"pr_event_webhooks"`) for independent rollout — costs
   nothing beyond one more string constant and one more `GetFeatureFlag` call;
   the map has no fixed key set to update. Preferable if this feature needs to
   ship/toggle independently of push-triggered session creation (e.g. to dark-launch
   CI-failure auto-reopen without touching the already-stable push-webhook path).

Either way, no new config infra is needed — this is a non-decision from a
stack-research perspective; it's a naming/scoping call for the architecture
phase, not a technical constraint.

## GitHub webhook payload schema versioning/compat concerns

- GitHub does not version webhook payloads via a header/field the way some
  APIs do (there's no `payload_version` field to check). Payload shape
  changes are additive in practice (new optional fields), and GitHub's stated
  policy is not to remove fields without a deprecation notice — but this repo
  has no dependency pinning that shape today (no SDK version to bump), so
  there's nothing to "version" in the Go-dependency sense. The risk instead is
  entirely in the hand-rolled field-extraction code: a locally-scoped
  extractor (per the `google/go-github` recommendation above) reading a field
  that's absent must degrade gracefully (`ok bool` return, as
  `extractGitHubRepoAndBranch` already does) rather than panic on a type
  assertion — same pattern to replicate for the new extractors.
- **Fork PRs**: `check_run`/`workflow_run` payloads from a fork's CI run may
  have an empty `pull_requests` array (GitHub's documented behavior — it can't
  always resolve which PR a check belongs to across fork boundaries). This is
  a real gap for Goal 2 ("invoke `AutoReopenForPRFix`... on a relevant event
  for a PR this instance is tracking") — matching webhook payload to a
  workflow's known PR may need a fallback lookup on the workflow's tracked PR
  branch/repo rather than only trusting the payload's `pull_requests[]` array.
  Flag this for the architecture phase; it's a payload-shape gap, not a
  dependency/stack concern.
- **`action` field discipline**: each event type carries multiple `action`
  values (e.g. `check_run` fires for `created`, `in_progress`, and
  `completed` — only `completed` is a terminal signal worth reacting to;
  `pull_request_review` fires for `submitted`, `edited`, `dismissed` — only a
  `submitted` review with `state == "changes_requested"` maps to
  `AutoReopenAfterFailedReview`). GitHub's webhook docs are the source of
  truth for the exact `action` enum per event type; this needs to be checked
  against GitHub's current docs during implementation, not inferred from this
  research pass, since GitHub does occasionally add new `action` values (most
  recently for newer event types) without a breaking-change notice.

## Routing target — confirmed interface shapes

- `session/backlog_lifecycle_pr.go:21-22`:
  ```go
  type PRFixSpawner interface {
      AutoReopenForPRFix(ctx context.Context, itemID string, fixContext string) error
  }
  ```
  implemented by `*BacklogService` (`server/services/backlog_service_triage.go:2018`).
- `AutoReopenAfterFailedReview` (`server/services/backlog_service_triage.go:1667`)
  is a `*BacklogService` method, not currently part of the `PRFixSpawner`
  interface — the new webhook handler will need either a second narrow
  consumer-defined interface for this method (per
  `.claude/rules/interface-pollution-checklist.md`'s "define where consumed"
  guidance) or an extended `PRFixSpawner`, decided at the architecture phase.
  Both paths take an `itemID string`, meaning the new handler needs to resolve
  a PR event's `repository.full_name` + PR number back to a tracked backlog
  `itemID` before it can call either method — that lookup path (likely via
  `session.WorkflowRepository` or a PR-tracking table, not yet identified in
  this research pass) is an open question for the architecture research
  dimension, not a stack/dependency question.

## Summary of dependency decisions

| Need | Decision | New dependency? |
|---|---|---|
| Parse `check_run`/`workflow_run`/`pull_request_review`/`issue_comment` payloads | Locally-scoped field extractors over `map[string]interface{}` (mirrors `extractGitHubRepoAndBranch`), reusing `readAndDecodeWebhookBody` | No |
| GitHub API/webhook typed SDK | None — `google/go-github` not present anywhere in repo (`go.mod`, `go.sum`, imports all checked, zero hits); not needed for inbound payload field reads | No |
| HMAC signature verification | Reuse `server/services/webhook_signature.go`'s `VerifyGitHubSignature` (stdlib `crypto/hmac`) | No |
| Dedup / audit trail | Reuse `TriggerFireEventRepository` + `readAndDecodeWebhookBody`/`persistTriggerFireEvent`/`claimAndFireTrigger` from `webhook_trigger_common.go` | No |
| Feature flag | Reuse `webhook_triggers` (broadest fit, already used by adjacent webhook-subsystem code) OR add a narrow new flag — either costs one `map[string]bool` key, no schema change | No |
| HTTP route | Reuse existing `POST /webhooks/github` route/handler; branch on `X-GitHub-Event` header (currently unchecked — must be added) | No |

**No new third-party dependencies are needed for this feature**, same
conclusion as the sibling `webhook-triggers` project. Everything is either
already built by that project (HMAC, dedup, route registration, feature flag
mechanism) or a small extension of existing hand-rolled JSON field extraction
(no `go-github` needed for inbound webhook parsing).
