# Research: Technology Stack — dynamic-rule-reload

## 1. Is `fsnotify` already a dependency?

Yes. `go.mod:16`:

```
github.com/fsnotify/fsnotify v1.9.0
```

Already used in 4 places (production code, non-test):

| File | Purpose |
|---|---|
| `session/history_watcher.go` | Watches `~/.claude/projects/` for new `.jsonl` history files |
| `session/unfinished/watcher.go` | Watches `.git/` dirs for repo-scan invalidation (`WatchDirWatcher.fsnotifyLoop`) |
| `session/unfinished/gogitstore/mmapwatch.go` | Watches a commondir's `objects/pack/` for repack staleness (`packWatchLoop`) |
| `session/mux/autodiscover.go` | (present, not read in detail — same import) |

No new dependency needs to be added. Reuse the existing `fsnotify.NewWatcher()` / `watcher.Add(path)` idiom; do not vendor a second file-watch library.

## 2. Debounce/coalescing pattern for editor atomic-rename saves

Two of the three watchers do **not** debounce at all (`history_watcher.go`, `watcher.go`'s `fsnotifyLoop`) — they just forward every matching event. `watcher.go` gets away with this because `Scanner.EnqueueRepo` itself is idempotent/cache-gated downstream (queueing a repo that's already scanned recently is a no-op), so raw multi-fire from atomic-rename saves is absorbed there, not at the fsnotify layer.

**The canonical debounce pattern for this task lives in `session/unfinished/gogitstore/mmapwatch.go`** (`packWatchLoop`, lines 65–106) — this is the pattern to copy for settings.json reload, since a manual reload / rebuild call (like `rebuildClassifier()`) is not naturally idempotent-cheap the way `EnqueueRepo` is, and editors doing atomic-rename saves on `settings.json` will fire multiple raw events for one logical save:

```go
const packWatchDebounce = 200 * time.Millisecond

func (s *SharedObjectStore) packWatchLoop(w *fsnotify.Watcher, stop chan struct{}) {
	defer func() { _ = w.Close() }()

	var timer *time.Timer
	var timerC <-chan time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		select {
		case <-stop:
			return
		case _, ok := <-w.Events:
			if !ok {
				return
			}
			if timer == nil {
				timer = time.NewTimer(packWatchDebounce)
			} else {
				timer.Reset(packWatchDebounce)
			}
			timerC = timer.C
		case <-timerC:
			timerC = nil
			if err := s.refreshIndexes(); err != nil {
				log.Warn("gogitstore: refreshIndexes failed", "commonDir", s.commonDirAbs, "err", err)
			}
		case _, ok := <-w.Errors:
			if !ok {
				return
			}
		}
	}
}
```

Shape: reset a single `time.Timer` on every raw event (coalescing bursts into one quiet-period trigger), fire the actual reload only when the timer elapses, clean exit via a `stop` channel select branch, `defer w.Close()`. The file's own doc comment explains *why* 200ms: "a single repack typically touches several files in quick succession (temp pack, temp idx, rename each into place, unlink the old pair)" — directly analogous to an editor's atomic-rename save of `settings.json` (write temp file, rename over original, which fires Create+Rename+Write in quick succession). Reuse this constant/rationale verbatim or adapt it for `settings.json`.

Also note the graceful-degradation convention shared by all three existing watchers: if `fsnotify.NewWatcher()` or `.Add()` fails (unsupported platform, path doesn't exist yet), log a warning and no-op rather than erroring out — don't block startup on file-watch availability.

## 3. Go version / stdlib alternatives

`go.mod:3` → `go 1.26.3`.

No stdlib file-watching primitive exists in Go (no `os.Watch` or similar); polling `os.Stat` mtimes is the only stdlib-only alternative, and it's strictly worse than what's already installed. `fsnotify` is already a first-class dependency with three working idioms in this codebase — no better-fit alternative to evaluate or introduce.

## 4. ConnectRPC pattern for a new RPC (using `DeleteApprovalRule` as the template)

Four touchpoints, confirmed by reading `DeleteApprovalRule` end to end:

**a. Proto messages** — `proto/session/v1/session.proto:1415-1422`:
```protobuf
message DeleteApprovalRuleRequest {
  string id = 1;
}

message DeleteApprovalRuleResponse {
  bool success = 1;
  string message = 2;
}
```
And the RPC declaration at `session.proto:151-152`:
```protobuf
// DeleteApprovalRule removes a user-defined auto-approval rule by ID.
rpc DeleteApprovalRule(DeleteApprovalRuleRequest) returns (DeleteApprovalRuleResponse) {}
```
Both are inside the same `session.proto` file that already declares `UpsertApprovalRule`/`ListApprovalRules`/`GetApprovalAnalytics` — a new `ReloadClaudeSettingsRules` (or similar) RPC belongs in this same file, next to the other approval-rule RPCs.

**b. `make proto-gen`** regenerates:
- `gen/proto/go/session/v1/session.pb.go` (Go messages)
- `gen/proto/go/session/v1/sessionv1connect/session.connect.go` (Go Connect handler interface/client)
- `web-app/src/gen/session/v1/*_pb.ts` (TypeScript bindings — tracked in git despite `.gitignore`, per existing repo instinct)

**c. Two-layer Go handler pattern** — implementation lives in `server/services/rules_service.go` on `*RulesService` (business logic: validate, mutate, call `rs.rebuildClassifier()`, log, build response):
```go
// server/services/rules_service.go:141-157
func (rs *RulesService) DeleteApprovalRule(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteApprovalRuleRequest],
) (*connect.Response[sessionv1.DeleteApprovalRuleResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}
	if err := rs.rulesStore.Delete(req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	rs.rebuildClassifier()
	log.Info("[RulesService] deleted rule", "id", req.Msg.Id)
	return connect.NewResponse(&sessionv1.DeleteApprovalRuleResponse{
		Success: true,
		Message: fmt.Sprintf("Rule %s deleted", req.Msg.Id),
	}), nil
}
```
`*SessionService` (which is the type actually registered against the generated Connect handler interface) has a one-line pass-through wrapper (`server/services/session_service.go:3097-3103`):
```go
func (s *SessionService) DeleteApprovalRule(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteApprovalRuleRequest],
) (*connect.Response[sessionv1.DeleteApprovalRuleResponse], error) {
	return s.rulesSvc.DeleteApprovalRule(ctx, req)
}
```
A new manual-reload RPC should follow this exact split: real logic in `RulesService` (or a new method there, since it already owns `rebuildClassifier()` and is the natural owner of a second-source reload), with a thin delegating method added to `SessionService`.

**d. Registration**: no separate `service.Register(...)` call to touch — `SessionService` is the single type registered against the generated `sessionv1connect` handler interface elsewhere in `server/server.go`; adding a method to `SessionService` that matches the interface (regenerated after the proto change) is sufficient — the interface satisfaction is compile-time enforced.

## 5. Frontend toast/notification pattern

No third-party toast library (no `react-hot-toast`, `sonner`, etc. in `web-app/package.json`). This repo has an in-house notification system:

- `web-app/src/lib/contexts/NotificationContext.tsx` — `NotificationProvider` + `useNotifications()` hook, exposing `addNotification(notification: Omit<NotificationData, "id" | "timestamp">)` (toast + history) and `addToHistoryOnly(...)` (history only, no toast/sound — used for informational events like `task_complete`).
- `web-app/src/components/ui/NotificationToast.tsx` — the toast UI component itself, rendered by the provider.
- `web-app/src/lib/types/notification.ts` — `NotificationData` / `NotificationHistoryItem` shape.
- `web-app/src/lib/notification-policy.ts` — `TOAST_STALE_MS`, `ACTIONABLE_TOAST_STALE_MS`, `isActionable` — staleness/priority policy already exists and should be reused rather than reinvented for a reload toast.

For "toast + log line on reload (auto or manual)" (in-scope item 4 of requirements.md): call `useNotifications().addNotification({...})` from wherever the reload result surfaces on the frontend (likely a manual-reload button handler and/or a push notification triggered by the backend event), and use the existing `log.Info(...)` convention (matching `rebuildClassifier`/`DeleteApprovalRule`'s `log.Info("[RulesService] ...")` style) for the backend-side audit log line — no new logging library needed.

## Root-cause note (confirmed, relevant to in-scope item 1)

`LoadClaudeSettingsRules()` (`server/services/claude_settings_parser.go:59`) has exactly one non-test reference in the entire repo: its own function definition. It is never called from classifier construction (`NewRulesService`, `server/services/rules_service.go:40`) or anywhere else in production code — confirmed dead code, matching the requirements.md finding. `rebuildClassifier()` (`rules_service.go:432-443`) already has a comment referencing "claude-settings rules" ("Keep seed rules and claude-settings rules; replace user rules") — the swap logic is *ready* for a claude-settings source to exist in `rs.classifier.Rules()`, it's just never populated. Wiring `LoadClaudeSettingsRules()` into `NewRulesService`'s initial classifier build (or an equivalent construction path) is the fix for in-scope item 1; git-blame on `claude_settings_parser.go` / `rules_service.go` would pin down when the wiring was dropped, but is out of scope for this stack-only research pass.
