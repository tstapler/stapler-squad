# Build vs. Buy — Session Cold-Start UUID Loss

Headline finding: nearly every piece this bug needs **already exists** in the
codebase, unused at the two call sites that need it. This is a wiring gap, not
a missing-capability gap. Bias toward reuse is not just the repo default here —
it's almost the entire fix.

## 1. JSONL parsing/discovery

**No JSONL/NDJSON library is vendored.** `grep -iE "jsonl|ndjson" go.mod go.sum`
returns nothing. All existing JSONL handling in this repo
(`session/history.go`, `session/history_detector.go`) is hand-rolled
`bufio.Scanner` + `encoding/json.Unmarshal` per line — plain stdlib.

More importantly, the exact function this bug needs **already exists**:

- [`session/history_detector.go:118` `ClaudeProjectDirName(projectPath string) string`](../../../session/history_detector.go) —
  replicates Claude Code's own cwd→directory-name encoding (every
  non-alphanumeric byte → `-`).
- [`session/history_detector.go:137` `(*HistoryFileDetector) DetectByPath(projectPath string) (*HistoryFileInfo, error)`](../../../session/history_detector.go) —
  does *precisely* "find the newest `.jsonl` under
  `~/.claude/projects/<encoded-path>/`, validate it's a real conversation
  (skip `agent-*.jsonl`, validate UUID format), return its UUID and path."
  Doc comment: *"does NOT require a live process, making it suitable for
  sessions whose tmux session is dead (e.g. after a reboot)"* — this is
  word-for-word the cold-start-revive scenario in the bug report.

**Claude Code CLI itself** (`claude --help`, checked locally) exposes:
- `-c, --continue` — "Continue the most recent conversation in the current
  directory"
- `-r, --resume [value]` — resume by session ID or interactive picker
- `--from-pr [value]` — resume linked to a PR

`--continue` could in principle replace file-scanning entirely for the
"resume the latest conversation for this cwd" case. But it doesn't return the
UUID to the caller, so stapler-squad couldn't persist it for its own
history-linking, checkpointing (`session/instance_checkpoint.go`), or search
resolution (`SetResolveConversationUUID` in
`server/services/session_service.go`) — all of which depend on holding the
UUID string, not just launching a resumed shell. Shelling out to a
non-deterministic/interactive-capable subcommand to *learn* a UUID we could
read directly off disk is strictly worse: extra process spawn, stdout
parsing, and no behavior stapler-squad doesn't already get for free from
`DetectByPath`.

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| Reuse `HistoryFileDetector.DetectByPath` (stdlib-backed, already written) | Already exists, already tested surface area, no new dependency, returns the UUID directly | None found | **Recommended** |
| New JSONL library dependency | N/A | Solves a problem stdlib + existing code already solves; ~30 lines of logic doesn't justify a dependency per `interface-pollution-checklist.md`'s spirit | **Not recommended** |
| Shell out to `claude --continue` for discovery | Uses upstream's own resume logic | Doesn't surface the UUID for persistence/checkpointing; adds a subprocess spawn to a hot revive path; less deterministic than reading the file directly | **Not recommended** |

## 2. Durable persistence

The atomic-write session storage mechanism referenced in
`.claude/rules/fix-flaky-tests-dont-defer.md` is real (`server/services/hook_injector_test.go`'s
`writeSettingsAtomic`), but session/instance persistence itself is ent-ORM-backed
(`session/storage.go`: `Storage.SaveInstances`, `SaveInstancesSync`,
`UpdateInstance`, backed by `session/ent`), not a hand-rolled file store.

Durable, immediate persistence of `ConversationUUID` **already exists** and is
already documented as solving this exact race:

- [`session/instance_claude.go:429` `SetClaudeConversationUUID`](../../../session/instance_claude.go) fires a
  `claudeSessionIDSavedCallback` whenever the UUID changes.
- [`session/instance_claude.go:458` `SetHistoryInfo`](../../../session/instance_claude.go) fires the same callback,
  with a doc comment stating the exact intent: *"a HistoryLinker-detected UUID
  is persisted to durable storage immediately rather than waiting on the next
  incidental full SaveInstances sweep ... a tmux pane killed before that sweep
  runs would otherwise resume with no conversation UUID to pass to --resume."*

This is the requirements doc's Acceptance Criteria #2, already built, for the
*capture* side. The gap is on the *consume* side: `startLocked` (instance.go
~878) and `start` (~1068) both call `i.HasClaudeSession()` directly with no
attempt to recover a missing UUID first — confirmed by reading both call
sites; neither calls `tryExtractConversationUUID`, `DetectByPath`, or any
other recovery path before branching on `HasClaudeSession()`.

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| Build on existing `SetClaudeConversationUUID`/`SetHistoryInfo` + ent-backed `Storage` | Already durable, already atomic (ent transactional writes), already fires on capture, zero new persistence code needed | None — this is exactly the mechanism the doc comment says it's for | **Recommended** |
| New dedicated small file/KV store for UUIDs | N/A | Would duplicate ent-backed `Storage`, split session state across two persistence mechanisms, contradict `interface-pollution-checklist.md` (unjustified new abstraction with no second use case) | **Not recommended** |

## 3. Path encoding scheme

`ClaudeProjectDirName` (session/history_detector.go:118, cited above) already
replicates Claude Code's cwd-encoding scheme and is already the single
producer used by both `Detect` (live-process path) and `DetectByPath`
(dead-process/reboot path). No second implementation exists anywhere in the
repo (`grep -rn "EncodePath\|encoded-path\|ClaudeProjectDirName"` finds only
this one definition and its call sites in the same file plus
`session/history_watcher.go`/`session/history_linker.go`, which consume it,
not redefine it).

**Verdict: Recommended — reuse `ClaudeProjectDirName` directly.** Writing a
second path-encoding implementation for this fix would violate the "no
speculative reimplementation" norm and risks drift if Claude Code ever changes
its encoding (one place to update, not two).

## 4. Fork or adapt existing "resume detection" logic

Yes — `session/history_detector.go` + `session/history_linker.go` +
`session/instance_claude.go`'s `tryExtractConversationUUID` together already
form a "resume detection" subsystem:

- `tryExtractConversationUUID` (instance_claude.go:308) already has the right
  two-path structure (fast: live pane's open FDs via `Detect`; fallback:
  `DetectByPath` when the pane is dead) — but its doc comment restricts it to
  callers holding `stateMutex` already (e.g. `SwitchWorkspace`), and it is
  **not currently invoked from either revive call site** in `startLocked`/`start`.
- `HistoryLinker` (history_linker.go) is a *background* poller/fsnotify-driven
  service that eventually correlates a session to its JSONL and calls
  `SetHistoryInfo` — but it runs on a 5s poll / backoff schedule, so it isn't
  guaranteed to have already populated `ConversationUUID` by the moment
  `startLocked`/`start` evaluates `HasClaudeSession()` immediately after a
  restart. This is consistent with the bug report's "racy/inconsistent across
  back-to-back restarts" observation.
- For UI surfacing (Acceptance Criteria #4), `server/notifications/store.go`
  already provides a durable, typed notification mechanism
  (`NotificationRecord` + `NotificationType` enum, e.g. existing
  `notifTypeApprovalNeeded`, `notifTypeAutoApproved`) that session lifecycle
  events already use to reach the UI without the user reading server logs.
  This is the natural fit for a new `NOTIFICATION_TYPE_COLD_START_FRESH`-style
  event rather than inventing a new UI channel.

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| Adapt existing `DetectByPath` + `SetHistoryInfo`/`SetClaudeConversationUUID` call, invoked synchronously at the two revive call sites before the `HasClaudeSession()` check | Minimal diff, reuses every piece that already exists and is already tested/documented for this exact purpose, keeps both call sites identical (Acceptance Criteria #5) | Requires care that `tryExtractConversationUUID`'s "caller must hold stateMutex" contract is respected, or a stateMutex-safe variant is used at both sites | **Recommended** |
| Reuse existing `NotificationRecord`/`server/notifications` store for the "started fresh" signal (Acceptance Criteria #4) | Already durable, already has a UI consumer, already has a type enum to extend | New enum value needed (small, additive proto/enum change) | **Recommended** |
| Write a new standalone "resume recovery" package from scratch | N/A | Duplicates `history_detector.go` + `instance_claude.go` logic wholesale for no new capability | **Not recommended** |

## Bottom line

- No new dependency is warranted anywhere in this fix — stdlib JSONL handling,
  the ent-backed `Storage`, `ClaudeProjectDirName`, `DetectByPath`, and
  `server/notifications` all already exist and each maps 1:1 onto one of the
  four sub-problems in the requirements doc.
- The actual code change is almost entirely **wiring**: call the existing
  synchronous recovery path (`DetectByPath` via a stateMutex-safe entry point,
  akin to `tryExtractConversationUUID`) at the two `HasClaudeSession()` branch
  points in `session/instance.go`, and emit one new notification type when
  recovery fails and a session truly starts fresh.
- This keeps the fix small and low-risk, consistent with this being a
  wiring/race bug rather than a missing-capability bug — and avoids exactly
  the kind of speculative new abstraction `.claude/rules/interface-pollution-checklist.md`
  warns against.
