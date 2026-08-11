# Build vs. Buy — cold-start-uuid-loss

Headline finding (VERIFIED by direct read of the four call sites named in
requirements.md, plus a live `claude --help` run): every piece this fix needs
already exists in the codebase, just wired to run in the wrong order or not
invoked at the two revive call sites. This is a reordering/wiring bug, not a
missing-capability gap — no new dependency, library, or persistence layer is
warranted.

## 1. Existing OSS library/framework for "detect/resume a Claude Code
   conversation from JSONL history"

**No JSONL/NDJSON library is vendored** — `session/history_detector.go` is
hand-rolled `os.ReadDir` + regex + stdlib `sort`, no third-party dependency.
The function the requirements doc names as the fix mechanism already exists
and does exactly the described job:

- [`session/history_detector.go:137`](../../../session/history_detector.go) `(*HistoryFileDetector) DetectByPath(projectPath string) (*HistoryFileInfo, error)`
  — scans `~/.claude/projects/<encoded-path>/` for the newest `.jsonl`,
  filters `agent-*.jsonl`, validates UUID format, returns the UUID + path.
  Doc comment: *"does NOT require a live process, making it suitable for
  sessions whose tmux session is dead (e.g. after a reboot)"* — word-for-word
  the cold-start scenario in this bug.
- [`session/instance_claude.go:308`](../../../session/instance_claude.go) `tryExtractConversationUUID()` already
  composes this correctly: fast path (live pane's open FDs via `Detect`),
  fallback path (`DetectByPath` when the pane is dead). Confirmed by reading
  lines 308–363: it is *not* a stub, it's a real two-path implementation.

**Does the `claude` CLI itself expose a better resume-detection primitive
this code should rely on instead of scanning JSONL?** Checked via `claude
--help` (run locally, output captured):

```
-c, --continue                        Continue the most recent conversation in
                                       ...
-r, --resume [value]                  Resume a conversation by session ID, or
                                       ...
--from-pr [value]                     Resume a session linked to a PR by PR
                                       ...
```

`--continue` ("resume most recent conversation in cwd") could in principle
replace path-scanning for the narrow "just resume something" case, and
`session/claude_command_builder.go`'s `ClaudeCommandBuilder.Build()`
(confirmed by reading lines 35–63) already shows the repo's existing pattern
for how `--resume <uuid>` gets appended to the launch command today — but
`--continue` does not hand the UUID back to the caller. stapler-squad needs
the UUID string itself, not just a resumed shell: it's threaded into
`instance_checkpoint.go` (conversation forking), `history_transfer.go`,
`agy_adapter.go`/`claude_adapter.go` (import/search), and the durable
persistence path (`SetClaudeConversationUUID`/`SetHistoryInfo`, see §2
below). Switching the launch invocation to `--continue` would still require
a `DetectByPath`-equivalent read of the JSONL afterward just to learn what
UUID got picked — strictly more work, plus a second subprocess spawn on the
revive hot path, for no capability gained.

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| Reuse `HistoryFileDetector.DetectByPath` / `tryExtractConversationUUID`, called earlier | Already exists, already exercised by existing tests (`history_detector_test.go`), zero new dependency, returns the UUID directly | Needs a stateMutex-safe call path if invoked before the lock is held (see §4) | **Recommended** |
| New JSONL/NDJSON parsing library | N/A | Solves a problem ~60 lines of stdlib already solve; violates the "no unjustified new abstraction" spirit of `.claude/rules/interface-pollution-checklist.md` | **Not recommended** |
| Switch launch to `claude --continue`/`--resume` interactive picker as the primary resume mechanism | Uses upstream's own resolution logic | Doesn't return the UUID to the caller (breaks checkpointing/history-transfer/search which all key off the stored UUID string); adds a subprocess spawn/parse step to a revive path that doesn't need one; non-deterministic if cwd has ambiguous history | **Not recommended** |

## 2. SaaS/managed API

Not applicable. This bug is entirely local process/filesystem/DB state
(`~/.claude/projects/*.jsonl`, in-memory `Instance.claudeSession`, the
ent-backed session store) — there is no hosted service in the loop for
conversation-UUID recovery.

## 3. LLM-generated bespoke detector vs. battle-tested library — extend vs.
   trust `--continue`/`--resume` semantics vs. persist to the existing ent DB

The requirements doc's "Alternatives Considered" section frames DB
persistence as a *possible* larger-surface fallback if the path-detector
approach proves insufficient. Direct code reading shows **this DB
persistence path already exists today** for the capture side — it is not a
hypothetical alternative, it's already-shipped, unused-early-enough
infrastructure:

- [`session/instance_claude.go:429`](../../../session/instance_claude.go) `SetClaudeConversationUUID` and
  [`:464`](../../../session/instance_claude.go) `SetHistoryInfo` both fire a `claudeSessionIDSavedCallback` the
  moment the UUID changes. `SetHistoryInfo`'s doc comment states the intent
  explicitly: *"a HistoryLinker-detected UUID is persisted to durable
  storage immediately rather than waiting on the next incidental full
  SaveInstances sweep ... a tmux pane killed before that sweep runs would
  otherwise resume with no conversation UUID to pass to --resume."*
- [`server/services/session_service.go:4046`](../../../server/services/session_service.go) `wireClaudeSessionIDCallback`
  registers exactly that callback: `inst.SetClaudeSessionIDSavedCallback(func() { s.storage.SaveInstances(...) })`.
- [`session/ent_repository.go:299-306`](../../../session/ent_repository.go) confirms the ent schema has a
  dedicated `ClaudeSession` entity (`SetClaudeSessionID`, `SetConversationID`,
  etc.) — this is transactional, not a hand-rolled file store.
- On process restart, [`session/instance_serialization.go:260-294`](../../../session/instance_serialization.go)
  confirms `HistoryFilePath` (and, via the `ClaudeSession` struct embedded in
  the same deserialize path) `ConversationUUID` are reloaded from persisted
  storage into the in-memory `Instance` before any revive decision runs.

So the correctness risk of "extending the bespoke JSONL detector further" is
lower than it first appears, because the fix does not need to invent new
persistence — durable storage of the UUID is already wired end-to-end on
capture, and already reloaded on process start. The actual gap, confirmed by
reading both call sites named in the requirements doc:

- [`session/instance.go:878-921`](../../../session/instance.go) (`startLocked`) and
  [`:1068-1127`](../../../session/instance.go) (`start`) both call `i.HasClaudeSession()`
  (a pure in-memory check, `instance_claude.go:269-273`) to decide
  fresh-vs-resume, **then**, only on the fresh-start branch, call
  `i.tryExtractConversationUUID()` *after* `i.pm().Start(startPath)` has
  already launched `claude` with no `--resume` (line 921 / 1127, confirmed
  by reading the surrounding block). Recovery is real, it just runs one
  branch too late to change the branch it could have prevented.
- The in-memory UUID being empty at decision time is exactly the racy window
  the requirements doc describes: `ClearConversationState()`
  (`instance_claude.go:278`) or the cold-restore blocks' own
  `i.claudeSession.ConversationUUID = ""` reset (`instance.go:910-913`,
  `:1118-1121`) can zero the field before a background poller
  (`HistoryLinker`, which runs on its own timer, not synchronously) or the
  next `tryExtractConversationUUID()` call has re-populated it — a second
  restart landing inside that window sees `HasClaudeSession() == false` even
  though a resumable JSONL is sitting on disk.

This means the correctness-risk comparison in the requirements doc is really
between two variants of the *same* bespoke detector, not detector-vs-trusted-primitive:

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| Move the existing `tryExtractConversationUUID`/`DetectByPath` call before the `HasClaudeSession()` branch (both sites) | Minimal diff; reuses code with existing unit test coverage (`history_detector_test.go`); doesn't touch persistence at all; directly closes the requirements doc's "0 fresh-starts when a resumable JSONL was present" target | `tryExtractConversationUUID`'s doc comment restricts it to callers already holding stateMutex (`instance_claude.go:302-304`) — must confirm both call sites hold the right lock, or add a lock-safe variant | **Recommended** |
| Trust `claude --continue`/interactive `--resume` picker instead of pre-checking | Delegates resume-target selection to upstream | Non-deterministic (interactive picker) when running under `-p`/headless launch inside tmux; doesn't return the UUID needed elsewhere in the codebase (see §1) | **Not recommended** |
| Add *new* persistence plumbing for the UUID (schema/storage changes) | N/A — the requirements doc itself scopes this out unless the path-fallback proves insufficient | Duplicates the already-shipped `SetClaudeConversationUUID`/`SetHistoryInfo` → ent `ClaudeSession` → reload-on-deserialize pipeline documented above; contradicts `.claude/rules/interface-pollution-checklist.md`'s "don't build a second mechanism for what one already does" | **Not recommended** — confirms the requirements doc's own scoping call that this should only be pursued if research shows the path fallback is insufficient; it isn't, the fallback exists and works, it's just invoked too late |

## 4. Fork or adapt an existing better-tested primitive (ent-backed
   persistence) rather than a new independent recovery path

Directly answered by §3: the ent-backed `ClaudeSession` persistence
(`session/ent_repository.go`, `session/ent/schema/claudesession.go`) is
already the durable store in play, already reload-tested via
`instance_serialization.go`, and does not need a parallel "new independent
recovery path" — the fix is to make the *existing* two recovery mechanisms
(durable DB reload on process start, and `DetectByPath` filesystem fallback
for the narrower in-process racy window) run in the right order relative to
the `HasClaudeSession()` decision, not to add a third.

For the requirements doc's "durable, user-visible signal" success metric
(§ Success Metrics, "started fresh — could not resume" marker inspectable via
the session events/status API, not just log grep), an existing typed,
durable mechanism is already in place to extend rather than invent:

- [`server/notifications/store.go`](../../../server/notifications/store.go) — `NotificationRecord` +
  `NotificationType` int32 enum (e.g. `notifTypeApprovalNeeded = 1`,
  `notifTypeAutoApproved = 13`, confirmed by reading lines 21-30), with
  durable storage, read/unread state, and a dedup mechanism
  (`findUnreadDuplicate`). This is the same pattern named in
  `feedback_document_ai_decisions_in_edge_cases` (self-heal/auto-close
  actions should post a visible record, not act silently) — directly
  answers the requirements doc's Open Question about whether an existing
  notification mechanism exists to hook into. It does; extend the enum with
  one new value rather than building a new UI channel.

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| Extend `server/notifications`' existing `NotificationType` enum + `NotificationRecord` store with a new "started fresh, could not resume" type | Already durable, already has a UI consumer wired for session-lifecycle notifications, additive-only enum change | None found | **Recommended** |
| Build a new session-event/status field from scratch for this one marker | N/A | Duplicates an existing, already-durable, already-UI-wired mechanism for no new capability | **Not recommended** |

## Bottom line

- No new dependency, library, or persistence layer is warranted anywhere in
  this fix. `HistoryFileDetector.DetectByPath`, `tryExtractConversationUUID`,
  the ent-backed `ClaudeSession` store (capture-and-reload already wired),
  and `server/notifications`' `NotificationRecord`/`NotificationType` each
  map 1:1 onto one of the four build-vs-buy sub-questions.
- The `claude` CLI's own `--continue`/`--resume` flags are not a better
  primitive to switch to: they don't return the UUID string the rest of the
  codebase (checkpointing, history-transfer, search resolution) depends on
  holding, and would add a non-deterministic subprocess step to a revive
  path that can resolve the same answer from a direct file read.
- The actual fix is wiring/reordering: call the existing
  `tryExtractConversationUUID`/`DetectByPath` recovery **before** the
  `HasClaudeSession()` branch at both `startLocked` (`instance.go:878-921`)
  and `start` (`instance.go:1068-1127`) instead of after committing to
  fresh-start, respecting `tryExtractConversationUUID`'s stateMutex
  contract, and add one new `NotificationType` for the unrecoverable case —
  consistent with the requirements doc's own Small appetite and "no new
  proto fields / no schema migrations" scope constraint.

## Note on a near-duplicate prior research artifact

`project_plans/session-cold-start-uuid-loss/research/build-vs-buy.md` (an
adjacent, differently-named project directory already present in the repo)
contains a build-vs-buy writeup for what is functionally the same bug
report. Its findings are consistent with the independent verification done
here (both reached the same four conclusions from separately reading the
same call sites). Phase 3 planning should treat `session-cold-start-uuid-loss/`
and `session-revive-uuid-loss/` as likely-duplicate SDD runs of this same
item and confirm with the user which project directory is authoritative
before continuing past research, to avoid two divergent implementation
plans for one bug.
