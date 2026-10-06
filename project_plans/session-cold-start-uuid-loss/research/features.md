# Research: Features & Edge Cases — Session Cold-Start UUID Loss

## 1. Other lifecycle events that touch `ConversationUUID` / `claudeSession`

The bug's two call sites (`session/instance.go:878-921` in `startLocked`, `:1068-1127`
in `start`) are not the only paths that read/clear/recover the conversation UUID.
Anything that changes must account for these:

| Event | File:line | What it does to `ConversationUUID` |
|---|---|---|
| `Restart()` (explicit restart, MCP/RPC-triggered) | `session/instance.go:1509-1560` | Captures the UUID before kill, then **deliberately clears it** if the session `waspaused` and its worktree is recreated (`:1550-1559`) — because the Claude project-dir encoding is path-keyed, and resuming with a UUID captured under the old path fails with "no conversation found." Feeds into the same cold-restore branch afterward. |
| `Hibernate()` → checkpoint write | `session/instance_hibernate.go:39-94`, `session/instance_checkpoint.go:86-90,115-120` | Reads `i.claudeSession.ConversationUUID` into the hibernation checkpoint (`hibernation.Checkpoint.ClaudeConvUUID`-equivalent) and into `CreateCheckpoint`'s `Checkpoint.ClaudeConvUUID` — a **separate** durable copy of the UUID from the one in `session/storage.go`. Two persistence paths that can drift. |
| `ResumeFromHibernation()` / `resumeFromHibernationLocked` | `session/instance_hibernate.go:97-145` | Calls `i.Start(false)`/`startLocked`, i.e. goes through the same buggy cold-restore branch. Hibernate→Resume is explicitly named in the bug report's restart-churn sequence ("inactivity-timeout restart, service restart, then hibernation, all within ~2 hours"). |
| `SwitchWorkspace` (directory/worktree/revision change) | `session/instance_workspace.go:75-90,109-200` | Pre-extracts the UUID (`:79-81`, calling `tryExtractConversationUUID()`), logs whether it found one (`:190-194`), then kills the tmux session and calls `startLocked(s, false)` (`:172`) for the no-VCS directory-change branch — a **third caller** of the exact same cold-restore code the bug report describes, not mentioned in the requirements' "two call sites." Any fix must cover this path too or it will silently diverge. |
| `HistoryLinker` background poll/fsnotify | `session/history_linker.go:157-345` | The async, off-critical-path UUID-capture mechanism. Runs on a 5s poll tick (`historyLinkerPollInterval`) plus fsnotify, with per-session exponential backoff (`historyLinkerBackoffBase`/`Max`, `:14-19`) and a "parked" state after 10 consecutive misses. **This is the racy component**: if a session restarts twice within one poll interval (or while backed off/parked), `HasClaudeSession()` can still be false when the second restart's cold-restore branch runs, even though a JSONL now exists. |
| `recoverFromStaleResume()` | `session/instance_claude.go:78-96` | Fires when the PTY exit tail matches `"No conversation found with session ID"` (`staleResumePattern`, `:17-28`, wired in `session/instance_controller.go:59-74`). Calls `ClearConversationState()` then `Start(false)` — i.e. **an existing precedent for "detect a bad/stale UUID, clear it, and restart fresh,"** but it reacts to a failed `--resume` attempt after the fact rather than validating before starting. |
| `wireCallbacks` registration gap (already fixed once) | `server/services/session_service.go:983-1004` | Comment at `:991-1000` documents a near-identical prior incident (confirmed live 2026-08-02): sessions created after boot were never registered with `HistoryLinker`, so they never got a UUID captured at all, and any restart cold-started fresh. Fixed by calling `s.historyLinker.AddInstance(inst)` in `wireCallbacks`. This is the same failure *shape* recurring — a UUID-capture path with a coverage gap — now via a timing race instead of a registration gap. |
| `SetHistoryInfo` → durable persistence callback | `session/instance_claude.go:464-499`, wired via `wireClaudeSessionIDCallback` (`server/services/session_service.go:4033-4040`) | Already persists to storage (`s.storage.SaveInstances`) as soon as the UUID is captured or changed — **AC2's "persist as soon as captured" is already implemented** for the async/HistoryLinker path. The gap is specifically the synchronous cold-restore branch reading `i.claudeSession` (in-memory) before this async capture has had a chance to run. |
| One-off session `RunWithResume` / `tryExtractClaudeSessionID` | `session/instance_claude.go:387-424`, `session/session_driver.go:796-813` | A **separate** capture path for `OneShot` (`-p` one-shot invocation) sessions: parses `session_id` out of `--output-format json` output rather than scraping JSONL files. Independent of `HistoryFileDetector`/`HistoryLinker`; worth checking whether the recovery fix needs to special-case `OneShot` sessions (their JSONL still exists under `~/.claude/projects/`, so path-based `DetectByPath` fallback should still work, but the reporter's bug was on a `~/oneoff/...` **interactive** one-off session, not `OneShot`/`-p` mode — confirm which). |

## 2. Existing multi-file / race-condition handling (already solved pieces)

Two mechanisms already exist that materially reduce this project's scope — the plan
phase should reuse them rather than re-invent:

- **"Most recently modified JSONL wins"** — `session/history_detector.go:131-199`
  (`DetectByPath`). Already scans `~/.claude/projects/<encoded-path>/`, filters out
  `agent-*.jsonl` and non-UUID basenames, and sorts by `ModTime` descending. This is
  the exact mechanism suggested in the requirements' "attempt UUID recovery from the
  newest JSONL" direction — it exists today, just isn't invoked *before* the
  `HasClaudeSession()` gate in the cold-restore branch.
- **Multiple sessions sharing one directory** — `session/history_linker.go:264-283`
  documents (and guards against) exactly the "which JSONL is *the* conversation"
  ambiguity: the "most recently modified" heuristic is wrong once another session has
  run in the same directory *after* this one was paused/hibernated, because that
  session's newer JSONL would silently overwrite the correct stored UUID. The guard:
  skip the path-based fallback fastpath for already-linked `Paused`/`Hibernated`/
  `Stopped` sessions, only run it for previously-unlinked or `Active` sessions. **Any
  new synchronous recovery call inside `instance.go`'s cold-restore branch needs the
  equivalent guard** — the current bug is precisely a "previously-unlinked, about to
  restart" state, but if the session had *ever* linked before and lost the in-memory
  value, blindly taking "newest mtime in the directory" could pick a different
  session's conversation for directory-type sessions with a shared path.

## 3. Edge cases the recovery design must handle

1. **Race: process just (re)started, hasn't written to its JSONL yet.**
   `tryExtractConversationUUID()` (`session/instance_claude.go:308-363`) is already
   called synchronously right after `pm().Start()` in both cold-restore branches
   (`instance.go:921`, `:1127`) specifically to close this window — but it's a
   single, synchronous attempt with no retry. If Claude's JSONL write happens even a
   few hundred ms after this call returns (a real possibility — process startup +
   working-dir resolution + first-turn write), detection still misses and the
   *already-started* fresh process now owns the directory, potentially never getting
   linked until the 5s `HistoryLinker` poll catches it. Recovery-before-start (the
   actual fix direction) doesn't have this race for the *previous* conversation's
   file, but the *new* file, once created, still needs correlating to record which
   UUID this Instance object is now actually running — same race, just shifted.
2. **A JSONL that's actively being written by a still-running, undetected process.**
   If two Instance objects (e.g. a hung old process plus a freshly cold-started one)
   are both live against the same project directory momentarily, "pick newest mtime"
   picks whichever wrote most recently — could pick the dying process's file instead
   of the new one, or vice versa, mid-transition. `pm().IsAlive()` should be checked
   as consistently as `DetectByPath`'s directory scan to avoid stealing another
   live instance's active conversation.
3. **Working directory changed between conversation starts.** Confirmed as an
   already-handled case via *deliberate UUID clearing*, not recovery:
   `Restart()`'s worktree-recreation branch (`instance.go:1550-1559`) explicitly
   clears the UUID because Claude's JSONL directory is keyed by the **encoded
   absolute path** (`ClaudeProjectDirName`, `history_detector.go:118-129`), so a UUID
   captured under path A is not resumable/discoverable under path B via path-based
   detection — `DetectByPath` looks in `GetEffectiveRootDir()`'s *current* directory
   only. A UUID persisted durably (survives across a path change) may still be a
   valid `--resume` argument to the Claude CLI itself if it doesn't require CWD to
   match project dir — **this is unverified and worth a spike**: does `claude
   --resume <uuid>` require the CWD to be the original project dir, or does it
   resolve the transcript by UUID regardless of CWD? If the latter, persisted-UUID
   recovery is strictly more resilient than path-scan recovery for this case; if the
   former, a persisted UUID for a stale path is not usable and should be treated
   like "no recoverable conversation."
4. **Corrupted / truncated / permission-denied JSONL, or unreadable
   `~/.claude/projects/`.** `DetectByPath` (`history_detector.go:146-150`) already
   treats `os.ReadDir` failure as "not found" (`return nil, nil`), not an error —
   consistent with AC3's "missing/corrupt falls back to fresh start" requirement.
   Confirm the same graceful-degradation holds for a JSONL file that exists but is
   zero-length or has a corrupt final line (Claude Code can be killed mid-write) —
   `isValidUUID(basename)` only validates the *filename*, not file readability/
   content; the existing code never actually opens/parses the JSONL for
   `DetectByPath`, so a truncated file is not distinguishable from a healthy one at
   this layer, which is fine for this bug (only the UUID/filename matters for
   `--resume`) but would matter if a future change tries to validate file integrity
   before trusting it.
5. **Stale UUID that Claude's backend has already expired/deleted.** Already handled
   reactively via `isStaleResumeExit`/`recoverFromStaleResume`
   (`session/instance_claude.go:17-28,78-96`, wired in
   `session/instance_controller.go:59-74`): detects `"No conversation found with
   session ID"` in the PTY exit tail and auto-restarts fresh. This is the
   AC3 "genuinely first-ever start... or missing/corrupt" fallback already
   implemented for the *post-launch* failure mode — the new recovery logic should
   not duplicate or race with this; it handles the case where recovery *thought* it
   found a resumable UUID but Claude's server disagrees.
6. **Non-Claude programs (Agy/Antigravity/Gemini).** `ClaudeCommandBuilder.
   isClaudeCommand()` (`session/claude_command_builder.go:71-86`) already gates
   `--resume` injection to `Program` basename `== "claude"`. The synchronous
   recovery call added to the cold-restore branch should have the same gate —
   otherwise every restart of a non-Claude session pays the cost of a
   `~/.claude/projects/` directory scan that can never succeed for it. `AgyAdapter`
   (`session/agy_adapter.go:18-30,49-90`) uses a completely separate discovery
   mechanism (`~/.gemini/antigravity-cli/history.jsonl`, keyed by workspace path) —
   out of scope for this fix, but confirms the UUID-recovery logic must not assume
   "any Instance" — it's Claude-CLI-specific.
7. **One-off sessions specifically (the reporter's case).** One-off working
   directories are generated via `namegen.GenerateUnique` under a dedicated base dir
   (`.claude/rules/session-creation-registry.md`'s one-off section) — i.e. unique
   per session, so the "multiple sessions share one JSONL directory" ambiguity in
   item 2 above should *not* apply to one-off sessions specifically, only to
   directory-type sessions where a user points two different session objects at the
   same folder. Worth confirming this assumption holds (grep for any one-off path
   reuse/recycling logic) since it changes how cautious the recovery heuristic needs
   to be for that session type.

## 4. Unstated user needs beyond "don't lose context"

- **Knowing *which* conversation got resumed.** Currently the only signal is a log
  line (`log.Info("cold restoring with --resume", ..., "uuid", ...)`,
  `instance.go:882,1076`) that's invisible to the end user. If recovery is added, a
  user who sees their session "just work" after a restart has no way to confirm it
  actually resumed the *right* conversation (vs. picked a wrong JSONL in a shared
  directory, per edge case 2) without checking server logs.
- **A way to tell if recovery guessed wrong.** Because `DetectByPath` is a heuristic
  ("newest mtime"), a user needs some low-friction way to notice "this doesn't look
  like my conversation" and manually recover — there's currently no UI affordance to
  pick a different history file or UUID for a session (checked: no `SetHistoryInfo`
  caller is exposed via any RPC/MCP tool). AC1-AC5 don't ask for a manual picker, but
  the requirements' own suggested direction #3 ("make cold-start-fresh louder") implies
  the inverse need is already felt — this generalizes to "tell me when you *did*
  something automatic to my conversation, not just when you gave up."
- **An audit trail across repeated restarts.** The bug report itself was diagnosed
  by the reporter piecing together *multiple* restart events ~2 hours apart from
  memory/logs. There's no structured history of "this session cold-started fresh at
  time T because reason R" visible anywhere in the UI today — only scattered
  `log.Info`/`log.Warn` lines. `Checkpoints` (`session/instance_checkpoint.go`) is
  the closest existing "session history" concept but is a user-invoked feature, not
  automatic.
- **Precedent for the "make it visible" mechanism**: `server/notifications/store.go`'s
  `NotificationRecord` (`:34-58`) plus the existing `NOTIFICATION_TYPE_WARNING = 8`
  enum value (`proto/session/v1/types.proto:783`) is a ready-made, already-wired
  session-scoped notification channel (has `SessionID`, `SessionScoped`, dedup by
  `(sessionID, notificationType)`) that the UI already renders — this looks like the
  natural target for AC4's "visible in the session's UI state/history" requirement,
  rather than inventing a new mechanism. `LifecycleEvent`/`fireLifecycleEvent`
  (`session/instance.go:69-88`, `session/instance_controller.go:130`) already carries
  a free-text `reason` string on `EventStarted` that a notification-emitting listener
  could key off of (e.g. reason `"cold-start-no-resume"`).

## 5. Comparable "resume vs fresh start" precedents in the codebase

- **`Restart()`'s worktree-recreation UUID clear** (`instance.go:1550-1559`) is the
  closest existing precedent for "we know resuming would be wrong, so deliberately
  don't" — same spirit as AC3 (prefer a clean fresh-start over a broken resume), just
  triggered by a known-bad condition (path mismatch) rather than an unknown one
  (UUID missing). The comment there is a good model for how to document the new
  recovery logic's tradeoffs.
- **`recoverFromStaleResume()`** (`instance_claude.go:78-96`) is the closest existing
  precedent for "detect a resume failure after the fact and recover automatically" —
  structurally the same shape the new logic needs (clear bad state → restart), just
  reactive instead of proactive.
- **`HistoryLinker.correlateSession`'s `force` parameter** (`history_linker.go:222-317`)
  is the existing pattern for "should I trust the currently-stored value or
  re-derive it," including the explicit carve-out for hibernated/paused sessions
  (edge case 2 above) — the new synchronous recovery call in `instance.go` is
  effectively a third caller of the same "detect and correlate" concept
  (`Detect`/`DetectByPath` are already shared by both `HistoryFileDetector`
  call sites: `tryExtractConversationUUID` and `HistoryLinker.correlateSession`) and
  should almost certainly call through the *same* detector/correlation function
  rather than a fourth bespoke implementation, to keep the "which JSONL wins" and
  "don't clobber a live different session" rules in exactly one place.
- **tmux/worktree reattachment** does not have a close analogue here — tmux session
  liveness is a binary alive/dead check (`pm().IsAlive()`), not a "pick the best of N
  candidates" problem, so there's no additional pattern to mirror from that side
  beyond what's already listed above.

## Key files for the implementation plan phase

- `session/instance.go:858-935` (`startLocked`) and `:1040-1140` (`start`) — the two
  call sites named in the requirements.
- `session/instance_workspace.go:75-90,155-180` — third, unnamed caller of the same
  cold-restore path via `SwitchWorkspace`'s no-VCS directory-change branch.
- `session/instance_tmux.go:248-279` (`initTmuxSession`) — builds the launch command
  (and thus whether `--resume` is included) *before* the `HasClaudeSession()` check
  runs later in `startLocked`/`start`; this ordering is the actual root cause
  mechanism, not just the log-line branch quoted in the requirements.
- `session/history_detector.go` (`Detect`, `DetectByPath`) and
  `session/history_linker.go` (`correlateSession`) — the two detection primitives to
  reuse for synchronous pre-start recovery instead of writing a new one.
- `session/instance_claude.go:308-363` (`tryExtractConversationUUID`),
  `:426-499` (`SetClaudeConversationUUID`, `SetHistoryInfo` — already-durable
  persist-on-capture).
- `server/notifications/store.go`, `proto/session/v1/types.proto:769-793` — candidate
  mechanism for AC4's UI-visible signal.
