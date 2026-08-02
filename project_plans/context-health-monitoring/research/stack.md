# Stack Research: context-health-monitoring

## 1. Loop detection ("N similar tool calls in a row")

No fuzzy-match/string-distance library exists in `go.mod` (checked full dependency
list — no Levenshtein, no `agnivade/levenshtein`, no `sahilm/fuzzy`, etc.). Adding one
is unnecessary: the requirement is "repeated tool call w/ similar args," and the
existing detection subsystem already solves an analogous problem (repeated-status
detection) with plain data structures, not fuzzy matching.

**Recommended pattern: exact/near-exact match + a small fixed-size ring buffer**, modeled
directly on `session/detection/events.go`'s `eventRing`:

```go
// eventRing is a fixed-capacity ring buffer of DetectionEvents.
type eventRing struct {
	mu     sync.Mutex
	events [EventRingCap]DetectionEvent
	head   int
	count  int
}
```
(`/home/tstapler/.stapler-squad/repos/github.com/tstapler/stapler-squad/session/detection/events.go:28-34`)

For loop detection, a much smaller ring (e.g. last 5-10 tool calls) storing a
normalized key — tool name + a cheap fingerprint of args (e.g. `tool_name +
truncated/hashed arg string`, not full Levenshtein distance) — is sufficient to catch
"same tool, same-ish args, N times in a row." Exact-match on a normalized key (trim
whitespace, truncate to first N bytes, or hash via `murmur3` — already vendored,
`github.com/spaolacci/murmur3`, used elsewhere in the repo for exactly this
kind of cheap fingerprinting) avoids O(n²) string-distance computation and keeps
comfortably inside the <5ms/chunk NFR.

**Do not add a new dependency for this** — no vendored fuzzy-match/Levenshtein lib
exists and the ring-buffer + exact/fingerprint-match approach is both idiomatic for
this codebase and cheaper than fuzzy matching.

## 2. Apology/confusion language detection

The `session/detection` package already does exactly this shape of problem —
regex-based `StatusPattern` matching — and is the pattern to extend, not reinvent:

- `session/detection/dtypes/dtypes.go:8-14` — `StatusPattern{Name, Pattern, Description, Priority}`
  struct, `yaml`-tagged for config-file loading.
- `session/detection/pattern_set.go:25-64` — `NewPatternSet` compiles all regex patterns
  once at construction (immutable after that, no lock needed — directly reusable
  pattern for a `ConfusionPatternSet`).
- `session/detection/pattern_set.go:66+` (`MatchLines`) — priority-ordered regex sweep
  over normalized text, returns first match's status/name/description.
- `session/detection/normalizer.go` — `PTYNormalizer.Normalize`/`SplitLines` already
  strips ANSI and collapses CR-overwrites before matching; reuse this rather than
  re-normalizing PTY output a second time.

**Recommended**: add a new pattern category (e.g. `Confusion []StatusPattern` in a
health-specific struct, or a sibling `PatternSet`-like type) with a small built-in
default list (regex fragments like `(?i)i apologize|i'm sorry|let me try (a)?nother
approach|that didn't work`) compiled once at startup exactly like `compile()` does
today. This is simple substring/regex, not NLP/similarity — consistent with how
every other status category in this file already works, and keeps the "no new
external network calls" and low-latency NFRs trivially satisfied.

## 3. go.mod dependencies to reuse (repo root `go.mod`)

Relevant existing deps, no new ones needed:
- `github.com/spaolacci/murmur3 v1.1.0` — cheap hashing, usable for arg fingerprinting
  in loop detection instead of storing/comparing full arg strings.
- `regexp` (stdlib) — already the basis of all detection in `session/detection/`.
- No ring-buffer package is vendored as a third-party library, but the repo already
  hand-rolls one (`session/detection/events.go`'s `eventRing`) — copy that shape, sized
  down, rather than adding e.g. `container/ring` (stdlib, but heavier API than needed)
  or a new module.
- `github.com/puzpuzpuz/xsync/v4` — present for concurrent maps, not needed here since
  per-session health state is naturally scoped to the single `StatusDetector`/session
  actor and doesn't need a concurrent map.

Confirmed via direct read of `/home/tstapler/.stapler-squad/repos/github.com/tstapler/stapler-squad/go.mod` (full `require` block) — no fuzzy-match, ring-buffer, or
sliding-window third-party package present.

## 4. ConnectRPC pattern for a new per-session computed field

Best, most recent, simplest model to copy: the **`detected_status` / `detected_context`
field pair**, already structurally identical to "ContextHealth status + reason":

- Proto (`proto/session/v1/types.proto`):
  ```protobuf
  DetectedStatus detected_status = 68;
  // Empty when detected_status is UNSPECIFIED.
  string detected_context = 69;
  ```
  (`proto/session/v1/types.proto:225,229-230`) — an enum status field plus a sibling
  string "reason/context" field, at adjacent field numbers. This is the exact shape
  requirements.md asks for ("new ContextHealth status+reason").
- Go population site (`server/adapters/instance_adapter.go:169-172`):
  ```go
  // DetectedStatus / DetectedContext: typed detection fields (fields 68–69).
  protoSession.DetectedStatus = detection.DetectedStatusToProto(statusInfo.ClaudeStatus)
  protoSession.DetectedContext = statusInfo.StatusContext
  ```
  Computed once per snapshot alongside `SubStatus` (see line 157: "Compute status info
  once for SubStatus + DetectedStatus + DetectedContext") — i.e. one computation feeds
  three proto fields, avoiding redundant detection passes. A `ContextHealth` +
  `ContextHealthReason` pair should follow the same pattern: compute once in the
  adapter's per-session snapshot path, assign both fields together.
- After adding fields: run `make proto-gen` (regenerates `session/gen/session/v1/*.go`
  and `web-app/src/gen/session/v1/*_pb.ts`), per repo convention.
- A second, heavier example (`DiffStats`, `proto/session/v1/types.proto:480` /
  `session/ent_repository.go`) is DB-persisted via ent edges — **not** the right model
  here, since context-health is described as ephemeral/computed-per-chunk, not a
  durable entity; `detected_status`/`detected_context` (computed fresh each snapshot,
  not persisted) is the closer analog.

## 5. Frontend badge pattern

**`StatusBadge`** (`web-app/src/components/sessions/StatusBadge.tsx`) is the existing,
directly-reusable component — do not build a new one:

```tsx
export function StatusBadge({ reason, detectedStatus, title, context }: StatusBadgeProps) {
  // maps enum -> { label, icon, variant }, renders icon + label + title tooltip
}
```
(`web-app/src/components/sessions/StatusBadge.tsx:77-105`)

It already takes an enum (`AttentionReason` or `DetectedStatus`) plus an optional
`context` string for the tooltip — exactly "status + reason with tooltip" from
requirements.md. `getDetectedStatusInfo`/`getAttentionReasonInfo`
(`StatusBadge.tsx:15-68`) are pure switch-based mapping functions to model a
`getContextHealthInfo(health: ContextHealth): StatusInfo` function off of — add a new
switch function alongside these two, with green/amber/red variants likely reusing
existing `styles.reasonVariants` (`complete`/`idle`-style green, `approval`/`warning`-style
amber, `error` red) from `StatusBadge.css.ts` rather than inventing new CSS variants.

Rendering site: `SessionCard.tsx:538,547` shows `StatusBadge` and the sibling
`SubStatusChip` both rendered conditionally on the card — `ContextHealth` badge should
be added as a third conditional element in this same render block, following the
existing "only show if non-default" suppression pattern already used for
`StatusBadge`/`SubStatusChip` (see the comment at `SessionCard.tsx:533-538`: "StatusBadge:
only shown when SubStatusChip has nothing to display").

No new badge component is needed — extend `StatusBadge.tsx` with a `contextHealth`
prop/variant, or add a small sibling function reusing its `styles` module.
