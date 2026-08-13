# Architecture Research: dynamic-rule-reload

## 1. Classifier merge/replace semantics (`pkg/classifier/classifier.go:379-417`)

`RuleBasedClassifier` is a single flat `[]Rule` slice behind a `deadlock.RWMutex`, sorted
descending by `Priority`. There is no per-source partitioning inside the classifier itself —
"seed", "user", and "claude-settings" rules all live in one slice and are distinguished only
by the `Rule.Source` string field (`classifier.go:375-376`).

- `NewRuleBasedClassifier()` (`:386-390`) seeds the slice from `SeedRules()` only.
- `AddRules(rules []Rule)` (`:403-408`) **appends** under the write lock and re-sorts. Additive,
  non-destructive — this is what `session_service.go:307` currently calls once at startup for
  user rules (`classifierObj.AddRules(userRules)`).
- `ReplaceRules(rules []Rule)` (`:393-400`) **swaps the entire slice** under the write lock —
  a single atomic pointer-style replace (whole new backing array). This is what
  `rebuildClassifier()` calls to hot-swap.
- `Rules()` (`:411-417`) returns a defensive copy under the read lock.

The classifier has **no concept of "source" at the API level** — `ReplaceRules` always
replaces everything. Any caller that wants to update just one source's rules must first read
`Rules()`, filter, splice in the new set for its source, and call `ReplaceRules` with the
merged result. This is exactly the pattern `rebuildClassifier()` already uses (see §3) — and
it is the reason two independent reload triggers (DB rules vs. claude-settings) can race.

## 2. Where classifier construction happens today, and where a watcher goroutine belongs

**Confirmed regression** (matches requirements.md's finding): `session_service.go:304-308`
builds the classifier with seed rules + `AddRules(userRules)` only. `LoadClaudeSettingsRules()`
(`server/services/claude_settings_parser.go:59`) has **zero call sites outside `_test.go`
files** (verified via repo-wide grep) — it is fully dead in the live server path. Root cause is
scope, not a bug in the function itself: `claude_settings_parser.go` was written and unit-tested
but the wiring step into `NewSessionServiceFromConfig` → classifier construction was never
added.

**Lifecycle pattern to reuse — do not invent a new one.** This repo has an established,
repeated idiom for exactly this shape of problem (a background fsnotify-driven watcher, owned
by the server, cancelled on shutdown), used by three independent subsystems:

| Component | File:line | Ownership pattern |
|---|---|---|
| `SetupManager.WatchFile(ctx, path)` | `server/auth/setup.go:122-183` | Watches parent dir (to catch file creation) + the file itself; select-loop on `ctx.Done()` / `watcher.Events` / `watcher.Errors`; handles `Write`, `Create`, and `Rename` (atomic write-then-rename) explicitly. **Closest structural analog** — single config file, not a whole repo tree. |
| `HistoryFileWatcher` | `session/history_watcher.go` | Same shape, watches Claude JSONL dir. |
| `Scanner.fsnotifyLoop(ctx)` | `session/unfinished/scanner.go:359-400` | Debounces bursts of fs events into one scan; falls back to tick-only polling if `fsnotify.NewWatcher()` fails (non-fatal, logged as `Warn`). |

**Startup wiring (`server/server.go:140-186`, `wireDepsIntoServer`)**: background components are
started here, not inside `NewSessionServiceFromConfig`/`BuildDependencies`, via
`go deps.<Component>.Start(serverCtx)` — e.g. `go deps.HistoryLinker.Start(serverCtx)`
(`:154`), `deps.PRStatusPoller.Start(serverCtx)` (`:145`), `deps.UnfinishedScanner.Start(serverCtx)`
(`:158-159`). `serverCtx` is `connCtx` from `newServerBase()` (`server/server.go:66-87`),
cancelled by `Server.Shutdown()` — this is the shutdown hook the requirements doc asked about.
Components are attached to `ServerDependencies` in `server/dependencies.go` via the `warren`
DI setter pattern (e.g. `warren.Set(w3, "HistoryLinker", sessionService.SetHistoryLinker, historyLinker)`,
`dependencies.go:903`).

**Recommended wiring for this feature**:
1. Add a small `ClaudeSettingsWatcher` type (e.g. in `server/services/claude_settings_watcher.go`),
   modeled directly on `SetupManager.WatchFile` — but watching up to 4 paths (global + global-local
   + project + project-local from `LoadClaudeSettingsRules`'s existing `settingsPath` list,
   `claude_settings_parser.go:70-79`), with a `Start(ctx context.Context)` method.
2. Construct it in `NewSessionServiceFromConfig` (`session_service.go:304-324`, right after
   `rulesSvc := NewRulesService(...)`) and call `LoadClaudeSettingsRules(projectDir)` once at
   construction time to fix the "not wired at all" bug immediately (this alone closes the
   requirements doc's regression item, independent of the fsnotify work).
3. Give `RulesService` (or `SessionService`) a `SetClaudeSettingsWatcher` setter, register it
   in `ServerDependencies` via the same `warren.Set` idiom used for `HistoryLinker` (`dependencies.go:903`).
4. In `wireDepsIntoServer` (`server/server.go`), add `go deps.ClaudeSettingsWatcher.Start(serverCtx)`
   next to the `HistoryLinker`/`UnfinishedScanner` starts (`:154-159`) — this ties the watcher's
   lifetime to server shutdown automatically, no new shutdown-hook plumbing needed.
5. **Project-path caveat**: `LoadClaudeSettingsRules(projectDir string)` takes a single project
   dir, but sessions run against many different worktrees/repos simultaneously. The current
   startup-only call already has this limitation (it presumably passes the top-level workspace
   root or none). The fsnotify watcher should watch the *same* path(s) the startup load already
   resolves — do not try to widen scope to per-session project settings; that is a separate,
   larger feature the requirements doc explicitly puts out of scope ("Generic POST
   /api/rules/reload for all sources" is out of scope, and multi-project watching is adjacent to
   that same expansion).

## 3. Race between two independent reload triggers — merge/locking design

`rebuildClassifier()` (`rules_service.go:432-443`) today:
```go
func (rs *RulesService) rebuildClassifier() {
    userRules := rs.rulesStore.ToRules()
    existing := rs.classifier.Rules()          // read snapshot
    var nonUser []classifier.Rule
    for _, r := range existing {
        if r.Source != "user" {
            nonUser = append(nonUser, r)         // keeps seed + claude-settings as-is
        }
    }
    rs.classifier.ReplaceRules(append(nonUser, userRules...))
}
```
This already correctly preserves `claude-settings` rules across a **DB-rule** reload, because
it filters by `Source != "user"` before replacing. The **read-modify-write over `Rules()` →
`ReplaceRules()` is not atomic as a whole** (only each individual call is atomic under the
classifier's own lock) — so if a claude-settings reload and a DB-rule reload race:

- T1 (DB reload) reads `existing` (seed+claude-settings A +user-old).
- T2 (claude-settings reload) reads `existing` (seed+claude-settings A+user-old), computes
  seed+claude-settings B, calls `ReplaceRules(seed+claude-settings B)`.
- T1 finishes computing `nonUser` from its *stale* snapshot (still claude-settings A) and calls
  `ReplaceRules(seed+claude-settings A+user-new)` — **clobbering T2's claude-settings B update**.

This is a classic lost-update race on a read-modify-write pattern layered on top of an
otherwise-atomic primitive. The fix is **not** to add locking inside `RuleBasedClassifier`
(it's already correctly atomic for what it does); the fix is to serialize the *read-modify-write
sequence* at the `RulesService` level, since that's where the composition logic lives.

**Proposed design**:
1. Add a single `sync.Mutex` (call it `rebuildMu`) on `RulesService`, held for the full
   read-`Rules()`-filter-`ReplaceRules()` sequence in both `rebuildClassifier()` (DB-rule path)
   and a new `rebuildClaudeSettingsRules()` (claude-settings path). This turns the two
   independent reload triggers into a single serialized critical section, eliminating the lost
   update — cheap because reloads are rare, human-triggered-or-fsnotify-debounced events, not a
   hot path.
2. Generalize the filter to be explicit about all three sources instead of `!= "user"`:
   ```go
   func (rs *RulesService) rebuildClassifier() {
       rs.rebuildMu.Lock()
       defer rs.rebuildMu.Unlock()
       userRules := rs.rulesStore.ToRules()
       existing := rs.classifier.Rules()
       var keep []classifier.Rule
       for _, r := range existing {
           if r.Source == "claude-settings" || r.Source == "seed" {
               keep = append(keep, r)
           }
       }
       rs.classifier.ReplaceRules(append(keep, userRules...))
   }

   func (rs *RulesService) rebuildClaudeSettingsRules(newClaudeRules []classifier.Rule) {
       rs.rebuildMu.Lock()
       defer rs.rebuildMu.Unlock()
       existing := rs.classifier.Rules()
       var keep []classifier.Rule
       for _, r := range existing {
           if r.Source == "user" || r.Source == "seed" {
               keep = append(keep, r)
           }
       }
       rs.classifier.ReplaceRules(append(keep, newClaudeRules...))
   }
   ```
   Making both filters symmetric (each explicitly keeps the other two sources rather than
   excluding just itself) avoids a latent bug where a fourth source added later silently gets
   dropped by whichever filter forgot to allow-list it.
3. `rebuildMu` belongs on `RulesService`, not `RuleBasedClassifier` — this is source-composition
   business logic (a `RulesService` concern per the interface-pollution checklist: don't push
   consumer-specific policy into the shared primitive), while `ReplaceRules`'s own lock stays
   scoped to protecting the slice itself.

## 4. Manual reload RPC — design modeled on `DeleteApprovalRule`

**Name**: `ReloadClaudeSettingsRules` (mirrors the source name in `Rule.Source`, avoids the generic
"reload everything" shape the requirements doc explicitly puts out of scope).

Files to touch, in the same order the `session-creation-registry.md`-style checklist implies:

1. **`proto/session/v1/session.proto`** — add next to `DeleteApprovalRule` (`:151-152`):
   ```protobuf
   // ReloadClaudeSettingsRules re-parses ~/.claude/settings.json and
   // <project>/.claude/settings.json and hot-swaps claude-settings-sourced rules.
   rpc ReloadClaudeSettingsRules(ReloadClaudeSettingsRulesRequest) returns (ReloadClaudeSettingsRulesResponse) {}
   ```
   and messages next to `DeleteApprovalRuleRequest`/`Response` (`:1415-1422`):
   ```protobuf
   message ReloadClaudeSettingsRulesRequest {}

   message ReloadClaudeSettingsRulesResponse {
     bool success = 1;
     int32 rule_count = 2;   // rules loaded after reload, for the toast/log message
     string message = 3;
   }
   ```
2. **`make proto-gen`** — regenerates `session/gen/session/v1/*.go` and
   `web-app/src/gen/session/v1/*_pb.ts` / `*_connect.ts`.
3. **`server/services/rules_service.go`** — new method next to `DeleteApprovalRule`
   (`:141-157`), calling `LoadClaudeSettingsRules` + the new `rebuildClaudeSettingsRules` from
   §3, then logging (per requirements item 4) exactly like the existing
   `log.Info("[RulesService] deleted rule", ...)` calls.
4. **`server/services/session_service.go`** — thin delegator, same shape as the existing ones
   at `:3094-3102` (`DeleteApprovalRule` delegates `return s.rulesSvc.DeleteApprovalRule(ctx, req)`).
   SessionService itself is the ConnectRPC handler for the whole `SessionService` proto service
   (confirmed: `service SessionService` at `session.proto:11` is one service, and
   `RulesService` — the Go struct — is not separately mounted; every one of its RPC methods is
   reached only via a same-named delegator method on `*services.SessionService*`, e.g.
   `session_service.go:3086,3094,3102,3110,3118,3126,3134,3142,4516,4524`).
5. **`server/mcp/tools_rules.go`** — new tool registration in `registerRulesTools`
   (`:21-62`), modeled on `delete_approval_rule` (`:55-61`), e.g.:
   ```go
   s.AddTool(
       mcpgo.NewTool("reload_claude_settings_rules",
           mcpgo.WithDescription("Re-parse ~/.claude/settings.json and project .claude/settings.json and hot-swap claude-settings auto-approval rules.")),
       h.reloadClaudeSettingsRules,
   )
   ```
   plus a `reloadClaudeSettingsRules` handler function calling
   `h.svc.ReloadClaudeSettingsRules(ctx, connect.NewRequest(&sessionv1.ReloadClaudeSettingsRulesRequest{}))`,
   mirroring `deleteApprovalRule` at `tools_rules.go:212-218`.
6. **`web-app/src/lib/hooks/useApprovalRules.ts`** — new `reloadClaudeSettingsRules` callback
   next to `deleteRule` (`:159-168`), same shape (`create(...Schema, {})` →
   `clientRef.current.reloadClaudeSettingsRules(req)` → `await refresh()` since, unlike delete,
   there's no single ID to optimistically patch — a full refetch is simplest and cheap given
   this is a manual, human-triggered action).
7. **UI button**: `web-app/src/components/sessions/ApprovalRulesPanel.tsx` (confirmed consumer
   of `useApprovalRules`) — add a "Reload from settings.json" button near wherever
   claude-settings-sourced rules are displayed/filtered (the panel already supports
   `sourceFilter` per `ListApprovalRulesRequest`, so a per-source action fits the existing UI
   grouping).

## 5. Linear flow (not an EventStorming table — this is a config-reload feature, not a
multi-actor domain)

```
settings.json edited (global or project)
        │
        ▼
fsnotify Write/Create/Rename event  (ClaudeSettingsWatcher, modeled on
        │                            server/auth/setup.go:122-183)
        ▼
debounce (short, e.g. 300ms coalescing window — mirrors
        │  session/unfinished/scanner.go:359-400's burst-coalescing fsnotifyLoop,
        │  needed because editors write via temp-file+rename, which fires 2-3 events per save)
        ▼
LoadClaudeSettingsRules(projectDir)   (claude_settings_parser.go:59 — already
        │                              correct, just needs a live caller)
        ▼
validate (implicit today: invalid regex patterns are skipped with a Warn log,
        │  claude_settings_parser.go:126-130 — no hard-failure path, by design,
        │  so a malformed settings.json degrades to "fewer rules" not "crash")
        ▼
rebuildClaudeSettingsRules(newRules)  → RulesService.rebuildMu-guarded
        │                                classifier.Rules() → filter → ReplaceRules()  (§3)
        ▼
log.Info + eventBus.Publish(system-level event, NOT session-scoped)
        │
        ▼
frontend NotificationToast  (web-app/src/components/ui/NotificationToast.tsx)
```

Manual trigger enters the same flow at the `LoadClaudeSettingsRules` step, skipping the
fsnotify/debounce stages (RPC handler calls it directly, synchronously, then returns
`rule_count` for the toast).

**Notification plumbing note**: the existing `NotificationService.SendNotification` RPC
(`server/services/notification_service.go:67-82`) requires a `session_id` — it's scoped to
per-session notifications and enforces a localhost-origin check, not a fit for a
server-wide "rules reloaded" toast. `pkg/events/types.go:12-30` already has one non-session
event precedent — `EventBacklogItemChanged` (`:30`) — alongside the session-scoped ones
(`EventSessionCreated` etc., `:14-24`) and `EventNotification` (`:26`). The reload event should
follow `EventBacklogItemChanged`'s precedent: a new `EventType` (e.g.
`"claude_settings_rules_reloaded"`) published on the existing global `eventBus`, with the
frontend's SSE event stream (whatever already surfaces `EventBacklogItemChanged` to the UI)
picking it up and feeding `NotificationToast`. This avoids inventing a new
notification-delivery mechanism — reuses the one general-purpose "system event → toast" path
that already exists for backlog changes, which is exactly analogous in shape (no session_id
target, applies globally to the open UI).

## 6. Prior architecture-review artifacts covering this code

`project_plans/ai-rule-generation/research/architecture.md` already documents the current
`RulesService` shape closely and is directly reusable, cited here instead of re-derived:

- `RulesService` field layout — `ai-rule-generation/research/architecture.md:5-11` (matches
  what §1/§4 above found directly in `rules_service.go:29-36`, confirming no drift since that
  doc was written).
- `classifier.SeedRules()` location and shape — `architecture.md:76` ("defined at line 738 of
  pkg/classifier/classifier.go").
- `rs.allRuleSpecs()` as the "user + seed + claude-settings, all as
  `RuleSpec`" aggregation point — `architecture.md:84,142,252` — same function read directly
  above at `rules_service.go:409-429`; no new behavior needed there since it already handles a
  live claude-settings source once the classifier actually contains one.

No prior architecture doc covers the **fsnotify watcher lifecycle** or the **reload-race**
question — those are net-new findings from this research (§2, §3), grounded in the
`server/auth/setup.go` / `session/unfinished/scanner.go` / `server/server.go:140-186` precedents
rather than in any existing project_plans doc.
