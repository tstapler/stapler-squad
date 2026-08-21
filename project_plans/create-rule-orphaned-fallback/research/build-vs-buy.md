# Build vs. Buy: create-rule-orphaned-fallback

## 1. Existing reuse candidates for a "treat unset as default" fallback

Searched `web-app/src/`, `session/`, `server/services/`, `pkg/classifier/` (via
`git grep` against `origin/main`) for prior art on this exact idiom
(`?? "no-match"`, `|| "no-match"`, or an equivalent shared helper). Two directly
relevant precedents exist, both **inline `??` at the call site**, not a shared
helper function:

- [`ReviewQueuePanel.tsx:751`](https://github.com/tstapler/stapler-squad/blob/main/web-app/src/components/sessions/ReviewQueuePanel.tsx#L751)
  (same file as the bug):
  ```tsx
  `${ESCALATION_REASON_EMOJI[queueItem.metadata["escalation_reason_category"] ?? ""] ?? ""} ...`
  ```
  Falls back to `""` for a missing category key, which itself misses the
  `ESCALATION_REASON_EMOJI` map and resolves to `""` again (no emoji) — a
  deliberate two-layer nullish-coalesce documented in the comment above the
  map (`ReviewQueuePanel.tsx:137-139`): *"An unrecognized/missing category
  falls through to no emoji via `?? \"\"`."*
- [`ApprovalAnalyticsPanel.tsx:93-101`](https://github.com/tstapler/stapler-squad/blob/main/web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx#L93-L101)
  — `ESCALATION_CATEGORY_LABELS` (`Record<string, string>`), with a comment
  instructing callers to fall back to the raw category string (`?? category`)
  for unmapped keys.

Both are **lookup-table fallbacks** (missing key → default *value*), not the
shape needed here (missing key → treat as a specific *known* category for a
boolean gate). No shared `isNoMatchOrUnknownCategory`, `withDefault`, or
nullish-coalescing utility function exists anywhere in `web-app/src/lib/` or
`web-app/src/utils/` — the only `Default`-named exports found
(`useSessionDefaults`, `createDefaultRegistry`/`getDefaultRegistry`/
`resetDefaultRegistry` in `web-app/src/lib/omnibar/detector.ts`) are unrelated
(session-creation form defaults, detector-registry singleton), not applicable
to a metadata-field fallback.

Go side: `pkg/classifier/escalation.go`'s `CategorizeEscalationRuleID` *is* the
existing "empty input → known default category" precedent
(`case "": return EscalationNoMatch`), but it operates on `RuleID` at
classification time, not on `EscalationCategory` after the fact — it's the
right *shape* of pattern but wrong *layer* (it already ran once, before the
value was persisted; there's no equivalent "re-derive from a stored, possibly-
absent field" helper in `server/services/approval_store.go` or
`session/review_queue_poller.go`).

**Conclusion:** no existing helper to reuse. The two nearest precedents
(`ReviewQueuePanel.tsx:751`, `ApprovalAnalyticsPanel.tsx:93`) confirm this
codebase's convention is an inline fallback expression at the point of use,
not a shared utility — so a fresh inline check (or a small colocated helper
function if the acceptance criteria's "explicit non-no-match list" logic
needs a name) is consistent with existing style, not a deviation from it.

## 2. Existing TypeScript union type for `escalation_reason_category`

None. `git grep` for a frontend enum/union covering the five category strings
(`no-match`, `explicit-rule`, `domain-age`, `secret-scan`, `unclassifiable`,
`unexpected`) found no such type. Both existing lookup tables key on bare
string literals typed as `Record<string, string>`:

- `ESCALATION_REASON_EMOJI: Record<string, string>` (`ReviewQueuePanel.tsx:140`)
- `ESCALATION_CATEGORY_LABELS: Record<string, string>` (`ApprovalAnalyticsPanel.tsx:93`)

The metadata bag itself (`queueItem.metadata`) is untyped
(`Record<string, string> | undefined` inferred from usage), and the backend
proto (`types_pb.ts:2231`) only documents the known values in a comment on
`escalationReasonCounts` — it does not export a TS union or const-array of the
literal values. The Go source of truth,
[`pkg/classifier/escalation.go`](https://github.com/tstapler/stapler-squad/blob/main/pkg/classifier/escalation.go)'s
`EscalationCategory` string-const type, has no generated/mirrored TS
equivalent (it's a plain Go string type, not a proto enum, so `make proto-gen`
doesn't produce one).

**Implication for the fix:** a helper like
`isNoMatchOrUnknownCategory(category: string | undefined): boolean` would have
to type its parameter as bare `string | undefined` (matching existing
convention) unless this fix also introduces a new union type — which the
requirements doc's non-goals section rules out ("Any change to the escalation
category taxonomy itself"). The minimal-diff approach is an inline array/set
literal of the five known non-"no-match" categories
(`explicit-rule`, `domain-age`, `secret-scan`, `unclassifiable`, `unexpected`)
compared with `.includes()`, matching the existing untyped-string convention
rather than inventing a union type this PR doesn't own.

## 3. No case for a new dependency

Confirmed no OSS/SaaS option applies:

- The entire fix is a boolean-gating change to one JSX conditional (and
  optionally a one-line Go default in `session/review_queue_poller.go` if the
  team prefers a backend-side fix — see requirements.md's acceptance criterion
  5, "no backend change is required unless the chosen fix approach needs
  one"). There is no parsing, validation, scheduling, or state-management
  problem here that a library would address — it is direct comparison against
  a small fixed string set already fully enumerated in
  `pkg/classifier/escalation.go`.
- No new runtime behavior class is introduced (no new async work, no new
  external call, no new persisted schema) — the fix only changes which
  existing button renders under an existing data shape.
- "Build custom (trivial)" is correct because: (a) the logic is ≤5 lines
  either as an inline `!==` chain, an `Array.includes()` check, or a `Set`
  membership test; (b) the full domain of possible values is closed and
  already named as Go constants in `pkg/classifier/escalation.go` — nothing to
  discover or configure; (c) introducing a dependency (e.g. a validation
  library, a "default value" utility package like `lodash.defaultto`) would
  add a supply-chain surface and bundle weight to replace a single `!==`
  comparison, which fails any reasonable build-vs-buy cost/benefit test.

No further OSS/SaaS research was performed beyond confirming the absence of
an applicable case, per the requirements doc's stated complexity (1) and
non-goals.
