# Research: Pitfalls & Risks — Session Cold-Start UUID Loss

Agent 4 of Phase 2 research. All line numbers verified by direct read on 2026-08-06 against
the current `main` checkout at `/home/tstapler/Programming/stapler-squad`.

## 1. The fix must run *before* `initTmuxSession()`, not just before the `HasClaudeSession()` log branch

This is the single most important structural pitfall, because it can make an otherwise-correct
recovery patch silently ineffective.

- `startLocked` (`session/instance.go:845`) calls `i.initTmuxSession()` at line **858** —
  *before* the `!firstTimeSetup` / `HasClaudeSession()` branch at line **878-885**.
- `initTmuxSession()` (`session/instance_tmux.go:249-279`) reads `i.claudeSession.ConversationUUID`
  **at that call** (line 255-256) and bakes it into the tmux launch command via
  `buildLaunchCommand`/`ClaudeCommandBuilder.Build()` (`session/claude_command_builder.go:35-63`).
  `Build()` only appends `--resume <uuid>` if `claudeSession.ConversationUUID != ""` at
  the time `Build()` runs (`claude_command_builder.go:47-49`).
- The `HasClaudeSession()` check at `instance.go:881` (and `:1072` in `start()`) only decides
  which **log line** to print — `i.pm().Start(startPath)` at line 902/1103 has *already*
  launched the process using whatever command `initTmuxSession()` built at line 858/(equivalent
  in `start()`, need to check its own init call), which ran before recovery would have a chance
  to run if inserted at the `HasClaudeSession()` check site.
- **Consequence for the fix**: any recovery logic (JSONL scan, reload from durable storage) must
  execute before `initTmuxSession()`'s command-string construction, not inside the
  `if i.HasClaudeSession() { ... } else { ... }` block. A patch that only adds recovery inside
  that `else` branch will recover the UUID into `i.claudeSession` too late — the already-started
  tmux process will have launched `claude` with no `--resume` flag, and setting the UUID
  afterward only affects the *next* restart, not this one. Verify this in the implementation
  review by checking the actual `claude` command line tmux received (e.g. via
  `tmux list-panes -F '#{pane_start_command}'` in a manual test), not just that
  `i.claudeSession.ConversationUUID` was non-empty at some point.
- Confirm the same ordering exists in `start()` (`instance.go:1023`) at its own
  `initTmuxSession()`/`buildLaunchCommand()` call site before wiring the fix there too (AC #5
  requires no divergence between the two call sites — see §5 below).

## 2. Directory-collision: recovering the "most recent JSONL" can silently attach the wrong conversation

This is a **pre-existing, partially-mitigated** hazard in this exact codebase, not a
hypothetical — it is directly on point for AC #1's "recover from disk" behavior.

- `HistoryFileDetector.DetectByPath` (`session/history_detector.go:137-199`) scans
  `~/.claude/projects/<encoded-path>/` and returns the **most recently modified** `.jsonl`
  file as "the" conversation for that path (`history_detector.go:189-193`, sort by `modTime`
  descending, take `candidates[0]`).
- `ClaudeProjectDirName` (`history_detector.go:118-129`) encodes a project's absolute path by
  replacing every non-alphanumeric byte with `-`. **Two different `Instance`s with the same
  effective root directory collide on the same encoded directory** — this is architecturally
  possible today: `GetEffectiveRootDir()` (`session/instance_worktree.go:166-173`) returns
  `i.Path` directly for any session without a git worktree (i.e. every `directory`-mode or
  `one_off`-mode-if-reused session), and nothing in the codebase enforces that `i.Path` is
  unique across `Instance`s. Multiple sessions opened against the same existing folder (a
  documented, supported session type — `directory`) will produce the **same**
  `~/.claude/projects/<encoded-path>/` directory containing JSONL files from *all* of them.
- `HistoryLinker.correlateSession` (`session/history_linker.go:231-317`) already documents this
  exact failure mode and *only partially guards against it*:
  ```
  // Skip this fallback for already-linked Paused or Hibernated sessions: the
  // "most recently modified" heuristic is wrong when other sessions have run in
  // the same directory after the pause, because their newer JSONL files would
  // replace the correct stored UUID with a different session's conversation UUID.
  ```
  (`history_linker.go:268-273`). The guard (`pathFallbackAllowed`, line 274-275) only excludes
  the path-based fallback for sessions that are **already linked** (`alreadyLinked == true`)
  AND in `Paused`/`Hibernated`/`Stopped` status. **A session with an empty in-memory UUID
  (`alreadyLinked == false`) — exactly the state this bug report is about — gets no
  protection at all**: `pathFallbackAllowed` evaluates to `true` unconditionally for it
  (line 274, the `!alreadyLinked ||` branch of the OR).
- **Consequence for the fix**: a naive "scan the directory, take the newest JSONL" recovery
  implementation (AC #1) inherits this exact gap. If session A (directory-mode, path `/foo`)
  goes cold with an empty UUID while session B (a *different* Instance, also pointing at
  `/foo`, e.g. a second directory-mode session on the same folder, or a leftover session from
  before a worktree was later added) has been actively writing a newer JSONL in the same
  encoded directory, A's revive will resume **into B's conversation** — the "wrong conversation
  UUID" risk called out explicitly in the task brief. This is worse than the current bug
  (silent fresh start): it silently splices two unrelated conversations together and the agent
  may act on B's context believing it's A's.
- **Mitigation to carry into planning**: recovery driven purely by directory scan (no other
  signal) cannot safely disambiguate two sessions sharing a path. Options: (a) only trust
  path-based recovery when exactly one candidate directory-owning session is known to exist
  (cross-reference the live session list, not just the filesystem); (b) prefer a durably
  *persisted* per-session UUID (AC #2) as the primary recovery source and treat directory-scan
  as a strictly lower-confidence fallback used only when no other session claims that UUID;
  (c) at minimum, cross-check the recovered UUID isn't already claimed by another loaded
  `Instance` (`session/storage.go` `LoadInstances`) before accepting it, mirroring the existing
  `HistoryLinker` awareness of the multi-tenant-directory problem — do not regress below the
  protection level `history_linker.go:268-275` already established for the *already-linked*
  case.

## 3. `ClaudeProjectDirName` is a reverse-engineered replica of Claude Code's own encoding — drift risk

- `ClaudeProjectDirName` (`history_detector.go:118-129`) is this repo's own guess at how the
  `claude` CLI encodes project paths into `~/.claude/projects/` directory names (replace every
  non-alphanumeric char with `-`). It is covered by `TestClaudeProjectDirName`
  (`session/history_detector_test.go:155-172`), but that test only asserts internal
  consistency with this repo's own implementation, not against a live/upstream Claude Code
  version's actual algorithm.
- If a future Claude Code CLI version changes its own encoding scheme (e.g. different
  character substitution, case folding, path normalization before encoding), `DetectByPath`
  degrades **safely** — it returns `nil, nil` (no match found, `history_detector.go:146-150`),
  which falls through to the existing "no resumable conversation" fresh-start path (AC #3's
  explicitly-preserved behavior) rather than picking a wrong file. This is the fail-safe
  direction, but it does mean the recovery feature (AC #1) could silently stop working after
  an unrelated `claude` CLI upgrade with no error surfaced anywhere — worth alerting on via the
  same visibility mechanism requested in AC #4 (see §6) rather than assuming "AC #1 recovers it"
  stays permanently true.

## 4. TOCTOU between detection and use is low-risk here, but not zero

- Between `DetectByPath` returning a `HistoryFileInfo{ConversationUUID, HistoryFilePath}` and
  `claude --resume <uuid>` actually being invoked, the JSONL file itself is not reopened or
  relied upon by this codebase for the resume decision — the CLI subprocess (`claude
  --resume <uuid>`) is handed only the UUID string and re-resolves the transcript itself at
  spawn time. So a rename/rotation of the *file* between detection and spawn is not directly
  fatal the way it would be if this code held a file handle open across that gap.
- The real TOCTOU is at a coarser grain: between `DetectByPath` reading `os.ReadDir` +
  `entry.Info()` (`history_detector.go:146,174`) and the `claude --resume` subprocess actually
  starting, the underlying conversation could be deleted (e.g. a concurrent `/clear` or manual
  cleanup) — `claude --resume <now-missing-uuid>` would then presumably itself either error or
  silently start fresh, outside this codebase's control. Confirm during implementation what
  `claude --resume <deleted-uuid>` actually does (VERIFY, not inferred) since AC #3's "existing
  fresh-start behavior is preserved" implicitly assumes this subprocess-level failure is
  distinguishable/handled, and it currently is not observed anywhere in this codebase's tests.

## 5. Two call sites must not diverge — this codebase has already shipped this exact bug shape once

- AC #5 explicitly requires `startLocked` (`instance.go:845`) and `start`
  (`instance.go:1023`) to apply identical recovery logic. This is not a hypothetical concern:
  the 2026-07-29 OOM incident (`~/.claude/projects/.../memory/project_2026_07_29_oom_session_leak_fix.md`)
  was caused by exactly this shape — two call sites (`archiveItemWorkSessions` in
  `server/services/backlog_service.go` and `reconcileTerminalItemSessions` in
  `session/backlog_lifecycle.go`) implementing the same cleanup logic independently, which
  drifted apart (one added review-role handling, the other didn't) and reintroduced the bug
  the first fix was supposed to close.
- The two `instance.go` sites here are *already* duplicated near-verbatim (compare
  `instance.go:878-921` to `instance.go:1068-1127` — the VNC/CDP setup, `pm().Start`,
  `RestoreWithWorkDir`, PTY attach, and the "clear UUID, re-detect via
  `tryExtractConversationUUID`" block are copy-pasted between the two). Adding the new recovery
  step to both copies independently, by hand, is exactly how they will drift again.
  **Recommendation for planning**: extract a single shared helper (e.g.
  `func (i *Instance) coldRestoreRecoverUUID(startPath string)` or similar) that both
  `startLocked` and `start` call, rather than pasting the new logic into both blocks a third
  time. Note `startLocked` takes `*instanceState` (actor-confined) while `start` is a plain
  `*Instance` method — check whether `start()` itself always executes inside an actor context
  (i.e. whether it's safe to share a single implementation) before assuming a common signature
  is trivial; if not, that asymmetry is itself worth flagging back to the architecture research
  agent.

## 6. Silent-fix-masks-the-real-bug: recovery is a safety net, not a substitute for fixing restart churn

- The requirements doc itself is explicit that `driverInactivityTimeout` (hardcoded
  `10 * time.Minute`, `session/session_driver.go:46`, used at `:414`) is the suspected trigger
  of the restart churn that creates the UUID-loss window, and is explicitly Out of Scope for
  this fix.
- Risk: once recovery (AC #1) and durable early persistence (AC #2) ship, the *symptom*
  (context loss) disappears, which removes the only visible signal that churn is happening at
  all. If recovery is silent (no metric, no distinguishable log signature between "clean
  --resume, no restart needed" and "restart churn, but the safety net saved us"), the churn
  frequency becomes invisible and the underlying watchdog problem could get worse unnoticed
  indefinitely.
- **Existing precedent for why silent success this way costs more than a log line**: the
  backlog stuck-review investigation
  (`~/.claude/projects/.../memory/project_backlog_stuck_review_investigation.md`) found that
  "stuck-item notify-once bookkeeping is in-memory only" (`stuckReviewNotified` map in
  `session/backlog_lifecycle.go`) reset on every service restart, silently losing the signal
  that something needed attention — a direct analogue to the risk of an in-memory-only "we just
  auto-recovered" flag here disappearing across the very restart churn it's meant to surface.
  Any new "cold start recovered/lost UUID" signal added for AC #4 should be durable (persisted,
  not just an in-process counter or unbuffered log line), or at minimum should increment a
  metric/counter that survives process restarts, so its frequency stays visible after this fix
  ships — not just a UI toast that nobody sees if it fires while the browser tab is closed.
- No existing `events.NewNotificationEvent`-style plumbing was found wired specifically for
  session-lifecycle transcript-recovery events (`grep` across `session/instance*.go` and
  `server/services/session_service.go` found `events.NewNotificationEvent` used for other
  session events at `session_service.go:1830`, `:3984`, `:4018`, but nothing yet for
  UUID-recovery/loss) — this is a new code path to add, not an existing one to hook into,
  which is itself worth flagging: get the severity/priority level and whether it's
  per-session-persisted (so a user who wasn't watching still sees it later) right the first
  time, per this repo's own `feedback_document_ai_decisions_in_edge_cases` memory note ("AI
  decisions in edge cases" — self-heal/auto-recover actions should post a visible, durable
  record, not act silently).

## 7. Write-amplification risk in "persist earlier" (AC #2) — existing pattern already does a full-row upsert per capture

- `SetClaudeConversationUUID` (`session/instance_claude.go:429-444`) and `SetHistoryInfo`
  (`instance_claude.go:464-499`) both fire `claudeSessionIDSavedCallback` on **every UUID
  change** (guarded by a no-op-if-unchanged check, `:434-437` and `:471-474` — this part is
  already correctly debounced).
- The callback, wired in `wireClaudeSessionIDCallback`
  (`server/services/session_service.go:4031-4040`), calls
  `s.storage.SaveInstances([]*session.Instance{inst})` synchronously
  (`session_service.go:4038`), which goes through `saveInstancesToRepo`
  (`session/storage.go:263-282`) → `EntRepository.Update`
  (`session/ent_repository.go:347-...`) — a **full transactional row upsert of the entire
  session's fields** (title, path, status, branch, worktree info, etc. — see the `SetXxx`
  chain starting `ent_repository.go:362`), not an incremental `conversation_uuid`-only write.
  This is an **existing** pattern, not something the fix introduces, but any new call site that
  triggers this callback more often (e.g. calling `SetHistoryInfo`/`SetClaudeConversationUUID`
  from inside the new recovery path on every revive attempt, or from a more aggressive
  proactive-capture loop meant to satisfy AC #2's "earlier" persistence) directly multiplies
  full-row DB writes. This is particularly perverse because the **exact scenario that triggers
  this bug is rapid restart churn** — the same window where a naive "persist immediately on
  every capture attempt" fix would fire the most additional full-row upserts, i.e. the fix's own
  overhead concentrates exactly where load is already elevated.
- Good news: because this already goes through the ent-backed SQL repository (not a raw
  `os.WriteFile`/JSON file), the specific "torn write / corrupt JSON" class of risk the task
  description called out (citing `TestWriteSettingsAtomic_ConcurrentWritesToSameSettingsPath_NeverProduceCorruptJSON`,
  `server/services/hook_injector_test.go:414-452`) does **not** directly apply to
  `SaveInstances` — that hazard is specific to the hooks-config file at
  `.claude/settings.local.json` (`internal/claudehooks/claudehooks.go`'s `mutate()`, using a
  `os.CreateTemp`-based atomic-rename pattern per `.claude/rules/ent-schema-generation.md`'s
  sibling doc), which is a *different* persistence path from session/UUID storage. **Do not
  add a new raw `os.WriteFile` for "durable early UUID persistence"** — the existing
  `SaveInstances`/ent-repository path is already the correct atomic-transaction primitive to
  reuse; a bespoke JSON-file side-channel for "just the UUID, written earlier" would be a
  regression to the exact naive-write class this repo has already hardened against elsewhere.
  If AC #2's "earlier/more durable" persistence needs a narrower/cheaper write than a full
  `SaveInstances` row upsert, that should be a new, explicitly incremental ent update (e.g. an
  `UpdateOne(sess).SetClaudeSessionID(uuid)`-only call), not a new storage mechanism.

## 8. Concurrency: unguarded direct-field writes in the actor vs. mutex-protected writes from `HistoryLinker`'s background goroutine

This is the most concrete Go-concurrency finding and is directly relevant because the recovery
fix's natural insertion point is inside this exact unguarded code.

- `claudeSession` and `claudeSessionIDSavedCallback` are documented as protected by
  `claudeSessionMu sync.RWMutex` (`session/instance.go:301-303`: "claudeSessionMu protects
  claudeSession and claudeSessionIDSavedCallback. Separate from mu to avoid holding the
  instance write lock during persistence I/O.").
- However, `startLocked` (`instance.go:845`) mutates `i.claudeSession` **directly, with no
  lock at all**, at lines **910-913** (`if i.claudeSession != nil { i.claudeSession.ConversationUUID = ""; i.HistoryFilePath = "" }`)
  and identically in `start()` at lines **1118-1121**. This relies on "actor-goroutine
  confinement" — the doc comment in `session/actor.go:22-26` establishes the convention that
  functions accepting `*instanceState` ("Locked" twins, like `startLocked`) "access Instance
  fields directly without stateMutex" because only one actor command runs at a time for a given
  `Instance`.
- **But `HistoryLinker.correlateSession`** (`session/history_linker.go:231-317`) runs on a
  **separate background goroutine** (`HistoryLinker.run`, `history_linker.go:172-185`, ticking
  every 5s, `historyLinkerPollInterval`) and calls `inst.SetHistoryInfo(...)`
  (`history_linker.go:316`) — which *does* take `claudeSessionMu.Lock()`
  (`instance_claude.go:465`). Instances are registered with `HistoryLinker` once, at session
  creation (`server/dependencies.go:836`, `server/services/session_service.go:1002`), and are
  **not** deregistered before a cold-restore `Start()` call and re-registered after — so
  `HistoryLinker`'s poll loop or its fsnotify-triggered `ScanAll()`
  (`history_linker.go:157-167`, fires on **any** new JSONL file anywhere under
  `~/.claude/projects/`) can run concurrently with `startLocked`'s unguarded direct write to
  the very same `i.claudeSession.ConversationUUID` field, during the exact revive window this
  bug report is about.
- This is a real data race by Go's memory model (one side takes a mutex, the other doesn't —
  the mutex provides no protection unless *all* writers/readers use it), though it may not have
  been caught by `-race` in `make test-race`/`make ci` yet if the specific interleaving hasn't
  been exercised by an existing test with real timing (background poller + concurrent
  restart). `checklocks` static enforcement (`Makefile:712-717`) is not wired for the
  `session/` package's `instance.go`/`instance_claude.go` (only `session/git`,
  `session/detection`, `session/artifacts`, `session/cdp`, `session/scrollback`, `session/mux`
  are checked, per the `checklocks` target's file list) — so this gap would not be statically
  flagged. `actor-field-guard` (`Makefile:758`) only enforces *where* direct field writes are
  allowed to exist (confined to `session/instance*.go` and `actor.go`), not *that* they're
  race-free against other lock-using accessors of the same field.
- **Consequence for the fix**: whatever new recovery logic is inserted at
  `instance.go:910-913`/`:1118-1121` should not add a third, different-again access pattern.
  Two safe directions: (a) keep using the established actor-confinement convention for the
  *actor-side* write (consistent with the surrounding code), and instead close the gap by
  having `HistoryLinker` route its updates for a session that's mid-revive through the actor's
  mailbox too (so all mutation of `claudeSession` for a given `Instance` is serialized through
  one path during that window) — larger change, likely out of scope; or (b) at minimum, replace
  the raw unguarded `i.claudeSession.ConversationUUID = ""` / `i.HistoryFilePath = ""` at those
  two sites with the existing `ClearConversationState()` helper
  (`session/instance_claude.go:278-296`), which already takes `claudeSessionMu.Lock()` **and**
  `i.mu.Lock()` correctly and rebuilds/publishes the snapshot — this at least makes the *clear*
  operation race-safe against `HistoryLinker`, even though full elimination of the actor
  vs. background-goroutine race for `claudeSession` as a whole would need a broader change than
  this bug fix's stated scope. Flag this explicitly to the architecture-review agent (Phase 2)
  and the pre-mortem (Phase 4) rather than silently leaving it as-is, since the new recovery
  code the AC's ask for will read/write this same field even more often than today.

## Summary of concrete file:line references

| Concern | File:line |
|---|---|
| `initTmuxSession` runs before `HasClaudeSession()` check | `session/instance.go:858`, `:878-885` |
| `--resume` only added if UUID non-empty at `Build()` time | `session/claude_command_builder.go:47-49` |
| `DetectByPath` "most recent JSONL wins" heuristic | `session/history_detector.go:189-193` |
| Directory-collision guard exists only for already-linked sessions | `session/history_linker.go:268-275` |
| `GetEffectiveRootDir` returns raw `i.Path` (no uniqueness) for non-worktree sessions | `session/instance_worktree.go:166-173` |
| `ClaudeProjectDirName` reverse-engineered encoding | `session/history_detector.go:118-129`, test at `history_detector_test.go:155-172` |
| Two duplicated call sites (drift risk) | `session/instance.go:878-921` vs `:1068-1127` |
| Prior duplicated-call-site incident (OOM) | memory: `project_2026_07_29_oom_session_leak_fix.md` |
| In-memory-only notify state lost on restart (analogue) | memory: `project_backlog_stuck_review_investigation.md` |
| UUID-save callback → full-row DB upsert per capture | `server/services/session_service.go:4031-4040` → `session/storage.go:263-282` → `session/ent_repository.go:347+` |
| Atomic-write precedent (different persistence path, not directly reusable) | `server/services/hook_injector_test.go:414-452`, `internal/claudehooks/claudehooks.go` |
| Unguarded direct field write vs. mutex-protected `HistoryLinker` writer | `session/instance.go:910-913`, `:1118-1121` vs `session/instance_claude.go:464-499`, `session/history_linker.go:231-317` |
| `claudeSessionMu` declaration | `session/instance.go:301-303` |
| Actor-confinement convention doc | `session/actor.go:22-26`, `:103-120` |
| `ClearConversationState` (correctly-locked equivalent, not used at the two sites above) | `session/instance_claude.go:278-296` |
| `checklocks` does not cover `session/instance*.go` | `Makefile:712-717` |
| `actor-field-guard` scope (where, not race-safety) | `Makefile:758` |
