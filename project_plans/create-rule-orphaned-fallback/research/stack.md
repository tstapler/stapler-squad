# Stack Research: create-rule-orphaned-fallback

All contents read via `git show origin/main:<path>` — the local worktree `main`
checkout is stale/diverged (PR #315 merged upstream, not present locally).

## Frontend: `web-app/`

From `web-app/package.json` (origin/main):

| Package | Version |
|---|---|
| `react` / `react-dom` | `^19.0.0` |
| `typescript` | `^5.9.3` |
| `@types/react` | `^19` |
| `jest` | `^30.2.0` |
| `jest-environment-jsdom` | `^30.2.0` |
| `ts-jest` | `^29.4.11` |
| `@testing-library/react` | `^16.3.0` |
| `@testing-library/jest-dom` | `^6.9.1` |
| `@testing-library/user-event` | `^14.5.2` |

Test runner is Jest (not Vitest), config at `web-app/jest.config.js`. Standard
`npx jest --no-coverage --testPathPatterns="ReviewQueuePanel"` invocation applies
(per repo `CLAUDE.md`).

## Backend: Go

`go.mod` (origin/main): `module github.com/tstapler/stapler-squad`, `go 1.26.3`.

### `server/services/approval_store.go` — `PendingApproval` struct (~L37-61)

```go
type PendingApproval struct {
    // ... fields elided ...
    EscalationCategory string // in-memory field, no json tag on this copy
    // (separate persisted struct, ~L50-61, has JSON tags:)
    EscalationReason   string `json:"escalation_reason,omitempty"`
    EscalationCategory string `json:"escalation_category,omitempty"`
    Orphaned           bool   `json:"orphaned"`
}
```

Both `escalation_reason` and `escalation_category` are `omitempty` on the
persisted struct — confirming the requirements doc's premise: JSON written
*before* PR #315 has no `escalation_category` key at all, and `omitempty` means
even *current* code never round-trips an explicit empty string; it's
indistinguishable on disk from "field didn't exist yet." `EscalationCategory`
is populated in 3 places (`~L160`, `~L320`, `~L396` — construction sites), all
copying `a.EscalationCategory` verbatim (no defaulting logic anywhere).

`ApprovalStore.loadFromDisk` (~L355) unconditionally marks all reloaded
approvals `Orphaned = true` — this is the existing signal already used for the
unrelated `metadata["orphaned"] = "true"` UI badge, so `Orphaned` is a
possible attachment point for a server-side default. `orphanedCleanupThreshold`
= 4h (~L65), matching the requirements doc's stated self-healing window. No
`approval_store_test.go` exists in the repo (confirmed via `git ls-tree`) — a
Go-level fix here would be the first test file for this struct.

### `session/review_queue_poller.go` — metadata population (~L825-857)

```go
type ApprovalMetadata struct {
    // ...
    EscalationCategory string // ~L67, plain field, no json tag (internal Go struct, not persisted)
}
```

The exact gating logic named in the requirements doc, confirmed verbatim:

```go
// ~L854-856
if a.EscalationCategory != "" {
    item.Metadata["escalation_reason_category"] = a.EscalationCategory
}
```

`item.Metadata` is `map[string]string` (constructed `~L834-835`:
`item.Metadata = make(map[string]string)`), matching the proto field type
below.

## Metadata type — proto + generated bindings

`proto/session/v1/types.proto`, `ReviewItem` message (~L560-575):

```protobuf
message ReviewItem {
  // ...
  // Additional metadata key-value pairs.
  map<string, string> metadata = 8;
}
```

Confirmed as plain `map<string,string>` — matches the Go
`map[string]string` and requires no additional wrapper type or optionality
handling on either side. Generated TS binding
(`web-app/src/gen/session/v1/types_pb.ts` ~L1022):

```ts
/**
 * Additional metadata key-value pairs.
 *
 * @generated from field: map<string, string> metadata = 8;
 */
metadata: { [key: string]: string };
```

Note: the generated field is `{ [key: string]: string }`, not
`Record<string, string> | undefined` — but `ReviewQueuePanel.tsx` accesses it
defensively via optional chaining (`queueItem.metadata?.[...]`) everywhere,
so the existing code already treats it as possibly-absent despite the
generated type not marking it optional (protobuf-es map fields are always
present as `{}` at minimum, but existing usage doesn't rely on that).

## Exact bug site — `web-app/src/components/sessions/ReviewQueuePanel.tsx`

Confirmed at **L844-845** (origin/main; file is 1464 lines total):

```tsx
{queueItem.metadata?.["tool_input_command"] &&
  queueItem.metadata?.["escalation_reason_category"] === "no-match" && (
  <Button
    ...
    onClick={(e) => {
      ...
      void generateRule({
        source: SuggestionSource.COMMAND_SAMPLE,
        commandSample: queueItem.metadata!["tool_input_command"],
        toolNameFilter: queueItem.metadata?.["tool_name"] ?? "",
      });
    }}
    data-testid={`create-rule-${queueItem.sessionId}`}
    ...
  >
    ✦ Create Rule
  </Button>
)}
```

This is the sole gate. The exact-match `=== "no-match"` needs to become an
"anything except the known non-no-match categories" check (per the
requirements doc's suggested fix) to also cover `undefined`/`""`.

## Known `EscalationCategory` enum values — `pkg/classifier/escalation.go`

Confirmed via `git show origin/main:pkg/classifier/escalation.go`, all 6 values
as `EscalationCategory string` consts:

| Const | String value | Meaning |
|---|---|---|
| `EscalationNoMatch` | `"no-match"` | No rule matched; classifier's default-escalate fallback |
| `EscalationExplicitRule` | `"explicit-rule"` | A named rule (seed/user/claude-settings) explicitly matched with `Decision: Escalate` |
| `EscalationDomainAge` | `"domain-age"` | Domain-age checker flagged a newly registered domain |
| `EscalationSecretScan` | `"secret-scan"` | Plaintext secret scanner flagged the command |
| `EscalationUnclassifiable` | `"unclassifiable"` | Command's executable could not be statically determined |
| `EscalationUnexpected` | `"unexpected"` | Internal classifier bug (`RuleIDUnexpectedDecision` sentinel) |

`CategorizeEscalationRuleID` (same file) is the single source of truth mapping
a `ClassificationResult.RuleID` to one of these — relevant if a server-side fix
is chosen (defaulting `EscalationCategory` at load time to `EscalationNoMatch`
would need to reuse/mirror this taxonomy, not invent a new one).

## Existing test infrastructure

- **Go**: `session/review_queue_poller_test.go` already has assertions on this
  exact code path (~L968-988): constructs an `ApprovalMetadata` with
  `EscalationCategory: "no-match"` and asserts
  `item.Metadata["escalation_reason_category"] == "no-match"`. No test
  currently exercises the empty/absent-category path in this file. No
  `server/services/approval_store_test.go` exists at all (confirmed absent via
  `git ls-tree -r origin/main --name-only`).

- **Frontend**: `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx`
  already has a `describe("ReviewQueuePanel — Create Rule button", ...)` block
  (~L313) and a `describe("escalation reason", ...)` block (~L942) with tests
  for `no-match` (L955, L1026 — button visible, `intent=secondary`) and
  `domain-age` (L1054 — button absent). **One directly relevant existing
  test** at ~L992-1005, `"shows the orphaned-approval fallback copy when
  escalation_reason is absent"`, builds an item with metadata that has
  `tool_input_command` set but **no `escalation_reason` /
  `escalation_reason_category` key at all** — i.e. exactly the orphaned-approval
  shape this bug is about — but it only asserts the reason-text fallback copy
  ("Reason not recorded..."), **not** Create Rule button visibility. This is
  the gap acceptance criterion 4 asks to close: no current test asserts the
  button *is* visible for this exact metadata shape (AC1), nor is there a
  parametrized check across all 5 non-no-match categories (AC3) — currently
  only `domain-age` is covered as a negative case.

## Implications for a fix

- **Frontend-only fix** (matches the "minimal-diff option" the requirements
  doc flags as preferred): change the L845 condition from an exact
  `=== "no-match"` allow-list to a deny-list against the 5 known non-no-match
  values from `pkg/classifier/escalation.go` (`explicit-rule`, `domain-age`,
  `secret-scan`, `unclassifiable`, `unexpected`) — everything else (including
  `undefined`/`""`) shows the button. This requires no proto change, no
  regeneration, and no Go change, since `metadata` is already a plain
  `map[string,string]` on both sides.
- A server-side alternative (defaulting `EscalationCategory` to `"no-match"`
  in `loadFromDisk`, gated on `Orphaned == true`) is also viable given the
  existing `Orphaned` flag, but touches `server/services/approval_store.go`
  (currently untested at the unit level — no `approval_store_test.go` exists)
  and duplicates the `omitempty`-driven absent-vs-empty ambiguity already
  present in `session/review_queue_poller.go`'s `!= ""` check. The frontend
  fix is more targeted since the bug is entirely observable/fixable at the one
  rendering site (`ReviewQueuePanel.tsx` L844-845).
