# Stack Research: subagent-spawn-tracking

## 1. Go regex capture-group idiom

No existing code in `session/detection/*.go` captures a numeric submatch today — the
`WaitingForAgent` patterns in `session/detection/detector.go` (lines 574-607) all match `\d+`
but discard it. The one existing capture-group extraction pattern in the package is in
`session/detection/approval.go` (`ApprovalDetector.Detect`, ~line 205):

```go
if match := pattern.compiled.FindStringSubmatch(line); match != nil {
    request := &ApprovalRequest{
        ...
        DetectedText:  match[0],
        ExtractedData: extractCaptureGroups(match, pattern.CaptureKeys),
        ...
    }
}
```

`extractCaptureGroups` pairs `match[1:]` positionally against a `pattern.CaptureKeys []string`
(named-key extraction without using `regexp.Regexp.SubexpNames()` / `(?P<name>...)` syntax —
no named capture groups are used anywhere in this codebase; positional `FindStringSubmatch` +
manual indexing is the established idiom).

**Recommendation for this feature:** follow the same idiom — `FindStringSubmatch`, not named
groups. Update the three patterns to add a capturing group around the `\d+`:

- `waiting_for_background_agent`: `` [✻◉✦]\s+Waiting for (\d+) (?:background agent|dynamic workflow) ``
- `shells_still_running`: `` (\d+)\s+shells?\s+(?:still\s+)?running ``
- `monitors_still_running`: `` (\d+)\s+monitors?\s+still\s+running ``

Then `match[1]` is the count string; convert with `strconv.Atoi` (already imported/used
elsewhere in the package — check `pattern_set.go`/`detector.go` imports before adding). Since
all three patterns feed into the single `StatusWaitingForAgent` detection, the code that walks
`ps.patterns.WaitingForAgent` (see `pattern_set.go:103`, `MatchStatus`) needs to be extended to
also return the matched count (or 0 if `Atoi` fails / group didn't match), propagated the same
way `Name`/`Description` already are today (as an additional return value or a field added to
whatever result struct the `Detect`/`MatchStatus` call chain returns — check `DetectFromLines`
call sites in `detector.go` around lines 660-720 for the current signature before adding a
field vs. a new return value).

## 2. Proto toolchain — exact regeneration command

- **Command:** `make proto-gen` (Makefile line 397) — wraps `buf generate proto`. It's
  stamp-gated (`$(PROTO_STAMP)`) and only regenerates when `proto/**/*.proto` is newer than the
  stamp file, `protoc-gen-es` binary is newer than the stamp, or the generated output files are
  missing. To force regeneration after a manual edit, either touch a `.proto` file or delete the
  stamp file (find `$(PROTO_STAMP)` definition earlier in the Makefile).
- **Config:** `buf.gen.yaml` (v2, `managed: enabled: true`, Go package prefix override to
  `github.com/tstapler/stapler-squad/gen/proto/go`). Three plugins:
  1. `buf.build/protocolbuffers/go` → `gen/proto/go` (`paths=source_relative`) — Go protobuf
     message types.
  2. `buf.build/connectrpc/go` → `gen/proto/go` (`paths=source_relative`) — Go ConnectRPC service
     stubs.
  3. local `web-app/node_modules/.bin/protoc-gen-es` → `web-app/src/gen`
     (`target=ts`, `ts_nocheck=false`, `keep_empty_files=true`) — TypeScript message types
     consumed by the React frontend.
- Also present: `buf.gen.go-only.yaml` (Go-only variant, presumably for contexts without the
  npm toolchain installed) and `buf.yaml` (module/lint config) — not needed for this feature
  unless `make proto-gen` is unavailable.
- **For this feature:** add `int32 subagent_count = <next-field-number>;` to the `Session`
  message in `proto/session/v1/types.proto` (it already carries `sub_status` at field 54 and
  `estimated_savings_mb` at 56 — use the next free field number, currently 57) — this is the
  message read by `session/instance_status.go`'s consumers and rendered by the web UI session
  card. Then run `make proto-gen`; this regenerates both
  `gen/proto/go/session/v1/session.pb.go` (Go) and `web-app/src/gen/session/v1/types_pb.ts`
  (TypeScript) in one command — no separate frontend codegen step exists.

## 3. Frontend badge/pill pattern

`web-app/src/components/sessions/SubStatusChip.tsx` is the exact existing analog and should be
extended rather than building a new one-off component:

- It already has a case for `SubStatus.WAITING_FOR_AGENT` that renders:
  ```tsx
  <span className={chipWaitingForAgent} role="status" aria-label="Waiting for agents"
        title="Claude is waiting for background agents to finish">
    ⏳ Waiting for Agents
  </span>
  ```
- Styling comes from `SubStatusChip.css.ts` (vanilla-extract, per ADR-009 / the
  `css-architecture` rule already in scope) — a set of `chip*` style exports per `SubStatus`
  variant, plus a `spinner` animation used by the `PROCESSING` chip.
- **Recommendation:** thread the new `subagentCount` (from the proto field) into
  `SubStatusChipProps` and interpolate it into the `WAITING_FOR_AGENT` case's label when
  `count > 0`, e.g. `` ⏳ Waiting for {count > 0 ? `${count} Agents` : "Agents"} `` — reusing
  `chipWaitingForAgent`'s existing style rather than adding a new CSS class. This satisfies the
  requirement's "⊕ N tasks" badge concept using the established chip idiom instead of a new
  component. (The exact glyph/wording — "⊕ N tasks" vs. extending the existing "⏳ Waiting for
  Agents" text — is a product decision for the plan phase; either way the *component and CSS
  pattern* to reuse is `SubStatusChip`/`SubStatusChip.css.ts`.)
- Other badge components in the directory (`GitHubBadge.tsx`, `SourceBadge.tsx`,
  `ReviewQueueBadge.tsx`, `OmnibarModeBadge.tsx`) follow the same
  `ComponentName.tsx` + `ComponentName.css.ts` (vanilla-extract) colocation convention — confirms
  this is the repo-wide pattern for any new small pill/chip, not just `SubStatusChip`.
- `SessionRow.tsx` is the consumer that renders `SubStatusChip` in the session list and
  currently filters out `IDLE`/`READY` before rendering (per `SubStatusChip.tsx`'s own doc
  comment) — worth checking when wiring up count display so the badge doesn't get filtered
  alongside those low-signal states.

## 4. xsync usage confirmation

`github.com/puzpuzpuz/xsync/v4 v4.5.0` is a direct dependency (`go.mod:25`) and is already used
in `session/instance_status.go`:

```go
// ponytail: xsync.Map replaces map+RWMutex — lock-free reads on the hot GetStatus path
controllers *xsync.Map[string, *ClaudeController]
```

This `xsync.Map[string, *ClaudeController]` is a **registry** (instance title → controller), not
a per-field mutable counter — `InstanceStatusManager.GetStatus()` loads the controller from this
map, then calls `controller.GetStatusAndIdleInfo()` to get the actual status data.

**Important nuance for the plan:** the count value itself would NOT live in the `xsync.Map`
directly. It flows through `ClaudeController.GetStatusAndIdleInfo()`
(`session/claude_controller.go:955`), which reads from `cc.statusCache.Load()` — an
`atomic.Pointer`-style copy-on-write cache keyed by a hash of the recent terminal tail
(`cc.statusCache.Load(); if sc != nil && sc.tailHash == h { ... }`), not a mutex and not
`xsync.Map`. The existing pattern is: "two independent lock-free loads replace the former
single RLock read" (comment at `claude_controller.go:983`). A new `subagentCount` field should
be added to whatever internal struct backs `sc` (the object returned by `cc.statusCache.Load()`,
containing `tailHash`, `status`, `desc` today) and returned as an additional value from
`GetStatusAndIdleInfo()` — following the atomic-pointer-cache pattern already established there,
not introducing a new mutex or reusing `xsync.Map` for a scalar. This is consistent with (but
more specific than) the requirements doc's note that "any new counter must respect that pattern,
not add a mutex" — the specific pattern to match is the `atomic.Pointer` cache in
`claude_controller.go`, with `xsync.Map` reserved for the higher-level controller registry in
`instance_status.go`.
