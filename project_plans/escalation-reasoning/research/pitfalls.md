# Pitfalls Research — Escalation Reasoning on Review Queue Items

Phase 2 research: what commonly goes wrong building this feature, grounded in the
current code at `server/services/approval_handler.go`, `server/services/approval_store.go`,
`session/review_queue_poller.go`, `server/services/analytics_store.go`, and the frontend
panels.

## 1. Concurrency / persistence pitfalls (`ApprovalStore`)

`ApprovalStore` (`server/services/approval_store.go:60`) guards `pending`/`bySession` with a
single `sync.RWMutex`. Every mutating path (`Create`, `Resolve`, `Remove`, `CancelSession`,
`CleanupExpired`) takes the write lock and then calls `persistToDiskLocked()` (line 291)
**while still holding `mu`** — so a new `Reason`/`RuleID` field on `PendingApproval` is safe
by construction as long as it's set on the struct *before* `Create()` is called (single
initial write, no separate mutator). There is no reader/writer race risk for the field itself
because `Create` already holds the write lock end-to-end (line 84-100) and `GetApprovalMetadataBySession`
(line 137) takes the read lock before copying fields out.

The one real risk is **forgetting to add the new field(s) to the `persistToDiskLocked` copy
loop** (line 296-310) — it manually re-lists every field from `PendingApproval` into
`PersistedApproval`; a new `Reason string` on `PendingApproval` that isn't also added to
`PersistedApproval` and to this copy loop will silently vanish on every disk write (compiles
fine, no error, field is just dropped). This is the same shape of bug as forgetting a case in
an exhaustive switch — nothing enforces the two structs stay in sync.

`persistToDiskLocked` already uses the correct pattern for partial-write safety: write to
`filePath + ".tmp"`, then `os.Rename` (line 326-335), which is atomic on POSIX filesystems —
a crash mid-write leaves either the old complete file or the new complete file, never a
truncated one. Adding a `Reason` field doesn't change this safety property since the whole
struct is marshaled via `json.MarshalIndent` in one shot (line 312) before any file I/O
happens. **No new pitfall here as long as the rename-based write path is reused unchanged.**

## 2. Backward-compatibility pitfalls (old JSON on disk)

Old `pending_approvals.json` written before this change will simply lack a `"reason"` /
`"rule_id"` key. `encoding/json.Unmarshal` into `PersistedApproval` (line 361,
`loadFromDisk`) leaves any missing field at its Go zero value — `""` for a `string` field —
which is safe and requires no special-casing, confirmed by the existing pattern: `Orphaned`
is force-set to `true` after unmarshal (line 382) but every other field is a plain value type
and unmarshals with defaults with no explicit nil-checks anywhere in `loadFromDisk`.

Two things would NOT default gracefully if implemented carelessly:
- **A `*string` (pointer) field instead of `string`.** Nothing in this codebase's pattern
  uses pointers for these value fields (`ID`, `SessionID`, `ToolName`, `Cwd`, etc. are all
  plain strings) — a `Reason string` (not `*string`) keeps the pattern and needs no nil guard
  downstream. If a future refactor changes it to a pointer, every consumer (`ReviewItem.Metadata`
  writer, the React panel) would need a nil check that doesn't exist today.
- **A category/enum-like string switched on without a `default` case.** The codebase already
  has a live precedent for this exact bug: `ComputeDailyBuckets` in
  `server/services/analytics_store.go:473-484` does `switch e.Decision { case "auto_allow": ...
  case "manual_deny": ... }` with **no `default`** — an unrecognized/zero-value string is
  silently dropped from all buckets, not reported as an error. The new 5-category breakdown
  (no-match / explicit-rule / domain-age / secret-scan / unclassifiable) is exactly this shape
  of code and will inherit the same footgun if written the same way: old `PersistedApproval`
  records loaded after restart have `Reason == "" ` and (if categorization derives the bucket
  from `RuleID` rather than a stored category) `RuleID == ""` too — that must map to a defined
  bucket (most naturally "unclassifiable") via an explicit case, not fall through unnoticed.
  `RecordFromResult` already treats `RuleID == ""` combined with `Decision == "escalate"` as a
  distinct "coverage gap" case (`analytics_store.go:396`) — that's the existing precedent to
  reuse rather than inventing new empty-string handling.

## 3. Goroutine-timing pitfalls (`ReviewQueuePoller` enrichment race)

`ReviewQueuePoller` runs its own loop at `PollInterval` (default 2s, backs off to
`SlowPollInterval` 8s when idle — `review_queue_poller.go:283-286`,
`DefaultReviewQueuePollerConfig` lines 43-51). `HandlePermissionRequest` in
`approval_handler.go` already special-cases this: after `h.store.Create(approval)` (line 371)
it explicitly calls `h.queueChecker.CheckSession(inst)` (line 383-388) to force an *immediate*
queue check rather than waiting up to 2-8s for the next poll tick — this exists specifically
so the review queue item appears with metadata already populated on first render, sidestepping
the race the research question raises. The enrichment itself happens inline inside the same
`buildSnapshot`/`shouldAdd` pass that creates the `ReviewItem` (`review_queue_poller.go:807-830`)
— metadata is populated **before** `rqp.queue.Add(item)` (line 833), not as a later patch, so
there is no intermediate "item exists but metadata is empty" state visible to a client polling
the RPC in between two poller ticks, *provided* `CheckSession` synchronously blocks until the
item is added. Confirm this synchronicity when implementing — if `CheckSession` only enqueues
work for another goroutine to pick up asynchronously, the immediate-check optimization doesn't
actually close the gap and the transient-empty-reason window reappears.

**Existing fallback pattern to copy** (`web-app/src/components/sessions/ReviewQueuePanel.tsx`):
every metadata-derived UI element is wrapped in `queueItem.metadata?.["key"] && (...)` —
optional chaining plus truthiness, so a missing/empty key renders nothing (not an "(empty)"
placeholder, not a loading spinner). See lines 718 (`context` suppressed once
`pending_approval_id` is set), 728 (`tool_input_command`/`tool_input_file`), 733 (`cwd`), 739
(`orphaned`), 818 (`tool_input_command` gating the "Create Rule" button). The new reason field
should use this same `queueItem.metadata?.["reason"] && (...)` conditional-render pattern —
consistent with every other transiently-missing field already in this file — rather than
inventing a distinct empty-state.

## 4. Proto/codegen pitfalls

The `proto-gen` Makefile target (`Makefile:397-412`) is a single `buf generate proto` call
that emits **both** Go (`gen/proto/go/`) and TypeScript (`web-app/src/gen/`) bindings from one
invocation — unlike the ent ORM generator, there is no flag equivalent to `--feature
sql/upsert` to get wrong; `buf generate` reads `buf.gen.yaml`'s plugin list and always
produces both language outputs together. So there is no "ran the Go plugin but not the TS
plugin" failure mode from a normal invocation.

The real risk is **staleness detection, not partial generation**: the target is gated behind
a timestamp check (`Makefile:399-403`) — `find proto -name '*.proto' -newer $(PROTO_STAMP)` —
and only regenerates if a `.proto` file is newer than `.proto-gen.stamp`. If a developer edits
`AnalyticsSummaryProto` and then manually runs `go build` without `make build`/`make
proto-gen`, the stamp file is untouched, the Go bindings in `gen/proto/go/` are stale, but
since Go bindings and TS bindings were generated from the *same* buf invocation the last time
this ran, they'd be **consistently stale together**, not skewed relative to each other — so
the practical failure is "the new proto field doesn't exist in either generated package yet"
(a straightforward Go compile error referencing an undefined struct field), not a silent
mismatch. Where skew *can* happen: committing only `gen/proto/go/*.pb.go` and forgetting
`web-app/src/gen/**/*_pb.ts` (or vice versa) in the same commit — nothing in CI currently
cross-checks that both generated trees came from the same `.proto` source hash, so a reviewer
must visually confirm both `gen/proto/go/` and `web-app/src/gen/` changed together in the diff.
Always run `make proto-gen` (not a hand invocation of `buf` or `protoc`) so the stamp file and
both output trees stay consistent, and `go build ./...` + `cd web-app && npx tsc --noEmit` (or
equivalent) after, to catch either side being stale before committing.

## 5. E2E flakiness pitfalls (AC8 — hook POST to `/api/hooks/permission-request`)

`HandlePermissionRequest` blocks server-side on `select` (`approval_handler.go:392-416`) until
one of: a decision arrives on `decisionCh`, `h.approvalTimeout()` elapses (default ~4 min,
strictly less than Claude Code's 5-min hook timeout per the comment at line 367), or
`r.Context().Done()` fires (client disconnect). **An e2e test that does `await
fetch('/api/hooks/permission-request', ...)` will hang for up to 4 minutes** waiting for that
response unless a decision is submitted through the review-queue approve/deny RPC from another
part of the same test — that's the correct pattern (fire the hook POST unawaited/backgrounded,
poll the review queue via the UI/RPC to observe the item, then trigger approve/deny, then
`await` the original fetch's resolution or just let it resolve independently). Firing the
POST without ever resolving it (test asserts on the queue item and finishes without
approving/denying) risks two distinct problems:
- **Client-side**: if the test's `fetch` promise is truly awaited and nothing ever resolves
  it, the test times out at Playwright's own timeout rather than the server's ~4 min one —
  still a slow, flaky failure. Fire-and-poll (don't `await` the initiating fetch on the main
  test thread) avoids this.
- **Server-side abandonment**: if the browser/test process is killed before the fetch
  completes and before Playwright's HTTP client sends a cancellation, the request context on
  the Go server may not observe `r.Context().Done()` promptly (depends on whether the transport
  actually tears down the TCP connection) — in the worst case the pending approval sits alive
  server-side until `CleanupExpired()` (`approval_store.go:233-270`) runs on its own sweep
  interval and its `ExpiresAt` passes. Because the e2e suite reuses a single isolated test
  server instance across the whole run (`tests/e2e/global-setup.ts` spawns one process for the
  entire suite, not per-spec), **an abandoned approval from this spec can outlive the spec and
  pollute the review queue for later specs** in the same run until it naturally expires
  (~4 min) — long enough to intersect with other specs' assertions on "queue is empty" or
  "queue has exactly N items." The test should always resolve every approval it creates
  (approve or deny) in a `finally`/`afterEach`, not rely on the timeout sweep, to keep the
  shared test server's queue state clean between specs.

No existing e2e spec exercises this hook endpoint today — `tests/e2e/onboarding-hook-install.spec.ts`
only tests the hook *installation* UI flow, not a live POST to `/api/hooks/permission-request` —
so there's no existing pattern in this repo to copy for the fire-and-poll shape; it will need
to be written from scratch, most likely via `request.post()` in Playwright fired without
`await`ing the response promise on the main flow, with an explicit resolve-and-await cleanup.

## 6. Security / data-leakage pitfalls (reason string vs. secret scanner)

Cross-checked `ScanForSecrets`/`FormatSecretDenyMessage` (`server/services/secret_scanner.go:53,66`)
and `redactedSecret` (`server/services/ai_interfaces.go:11`) against how each of the 3
non-secret-scan escalation paths builds its `Reason`:

- **No-match** (`pkg/classifier/classifier.go:526`): fully static string, `"No matching rule;
  escalated for manual review."` — no interpolation, zero leak risk.
- **Explicit-rule** (`classifier.go:512-515`): `Reason: rule.Reason` — a static,
  rule-author-authored string baked into the rule definition (e.g. `"Writing to .env files
  risks leaking or corrupting secrets."`, `"Deleting the root or home directory would cause
  irreversible data loss."` — lines 752, 779). These never interpolate the actual command
  text, so this path is safe by construction *for top-level rule matches*.
  **However**, the sub-command-splitting branch of the same classifier (multi-command strings
  joined by `&&`/`|`/`;`) DOES interpolate raw command text into the reason:
  `fmt.Sprintf("Sub-command %q: %s", sub.Raw, result.Reason)` (line 588) and
  `fmt.Sprintf("Sub-command %q has no matching allow rule; escalated for manual review.", sub.Raw)`
  (line 608) both embed `sub.Raw` — a literal substring of the original command — into the
  persisted reason. This is the one place a reason string echoes user-controlled command text
  outside the metadata's existing `tool_input_command` field.
- **Domain-age** (`approval_handler.go:248`): `fmt.Sprintf("Domain %q was registered within
  the last %d days...", domain, threshDays)` — only interpolates the extracted registered
  domain (via `ExtractDomainsFromCommand`/`publicsuffix.EffectiveTLDPlusOne`,
  `domain_checker.go:75-95`), never the full command string. Low risk — a domain name isn't a
  secret-scanner target class.

**Net assessment**: the sub-command reason (`sub.Raw` interpolation) is the only place a new
`Reason` field could carry command text the secret scanner's regex patterns missed (the
scanner runs once, up front, on the *full* `payload.ToolInput["command"]` string at
`approval_handler.go:207-208`, before the classifier ever runs — so anything in `sub.Raw` was
already covered by that same full-string scan, since `sub.Raw` is a substring of the string
that was scanned). The realistic residual risk is therefore not "unscanned text" but "a secret
in a form the regex patterns don't recognize" (custom internal token formats, anything not
matching the known pattern list) — which is a pre-existing gap in `ScanForSecrets`, not
something this feature introduces or worsens.

Also worth noting for scoping: **this is not new exposure surface at the persistence layer.**
`PersistedApproval.ToolInput` (`approval_store.go:47`) already disk-persists the raw,
unredacted command for every escalated approval today (no-match/explicit-rule/domain-age all
reach `store.Create` with the original `payload.ToolInput` unmodified — only the secret-scan
auto-deny path sanitizes before analytics, and it never reaches the store at all). Likewise
`AnalyticsEntry.Reason` (`analytics_store.go:29`) already persists classifier reason strings
to the analytics DB today via `RecordFromResult`. Adding `Reason` to `PendingApproval`/
`PersistedApproval`/`ReviewItem.Metadata` duplicates information that's already persisted
elsewhere in this codebase, rather than creating a new leak vector — the AC6 "plain text via
existing CSS class" requirement means it *also* becomes newly visible in the UI, which is the
actual net-new exposure (screen visibility, not disk persistence).
