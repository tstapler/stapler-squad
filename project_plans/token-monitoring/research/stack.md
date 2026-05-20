# Stack Research — Token Monitoring

## JSONL Storage

**Location:** `~/.claude/projects/<encoded-path>/<session-uuid>.jsonl`

**Path encoding:** Claude encodes the project path by replacing every non-alphanumeric character with `-`. Implemented in `session/history_detector.go` as `ClaudeProjectDirName(projectPath string) string`. Example: `/Users/alice/myproject` → `-Users-alice-myproject`. This is NOT a hash — it is a simple character-replacement encoding. Already implemented and tested in the codebase.

**Already-linked infrastructure:** `session/history_linker.go` + `session/history_watcher.go` correlate running sessions to their JSONL files. `HistoryLinker` sets `Instance.HistoryFilePath` and `Instance.ConversationUUID` via `inst.SetHistoryInfo(uuid, filePath)`. This means for any active session, we can find its JSONL file without path-scanning.

**JSONL message types observed** (sampled from `~/.claude/projects/-Users-tylerstapler/`):
- `permission-mode` — session config record
- `file-history-snapshot` — file backup state
- `user` — human turn messages
- `assistant` — model responses (contains token usage)
- `attachment` — skills/tool listings injected into context
- `queue-operation` — approval queue events
- `last-prompt` — prompt detection records
- `system` — system-level messages

**Token usage fields** (on `assistant` message type, inside `.message.usage`):
```json
{
  "input_tokens": 3,
  "cache_creation_input_tokens": 0,
  "cache_read_input_tokens": 39333,
  "cache_creation": {
    "ephemeral_5m_input_tokens": 0,
    "ephemeral_1h_input_tokens": 0
  },
  "output_tokens": 591
}
```
Fields: `input_tokens`, `output_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`. The `cache_creation` nested object contains `ephemeral_5m_input_tokens` and `ephemeral_1h_input_tokens` for extended cache TTLs.

**Model field:** `assistant.message.model` — e.g., `"claude-sonnet-4-6"`

## Chart Library

**Current status:** No charting library is in `web-app/package.json`. The dependencies include Next.js 15, React 19, @connectrpc/connect, @vanilla-extract/css, lucide-react, redux toolkit, fuse.js, @radix-ui. No recharts, Chart.js, D3, or any visualization library.

**Recommendation: recharts**
- ~200KB gzipped, React-native API (renders SVG via React components)
- Well-maintained, responsive by default, works with vanilla-extract theming
- Alternative: lightweight custom SVG charts if bundle size is a concern (session list badges only need a number, not a chart)
- For the full `/insights` dashboard: recharts `LineChart`, `BarChart`, `PieChart` cover all required chart types

## ConnectRPC Service Pattern

**Template:** `server/services/config_service.go`

```go
type ConfigService struct{}

func NewConfigService() *ConfigService {
    return &ConfigService{}
}

func (cs *ConfigService) GetClaudeConfig(
    ctx context.Context,
    req *connect.Request[sessionv1.GetClaudeConfigRequest],
) (*connect.Response[sessionv1.GetClaudeConfigResponse], error) {
    // implementation
    return connect.NewResponse(&sessionv1.GetClaudeConfigResponse{...}), nil
}
```

**Registration:** Services registered in `server/server.go`. Pattern: implement RPC methods, register with ConnectRPC mux.

**Streaming RPCs** (for live data): Pattern is `rpc WatchSessions(WatchSessionsRequest) returns (stream SessionEvent) {}` in proto → implemented as server-streaming in Go with `connect.ServerStream`.

**Frontend hook pattern:** `web-app/src/lib/hooks/useSessionService.ts`
- Uses `createClient` from `@connectrpc/connect`
- Uses custom `createWatchTransport` for streaming
- Imports from `@/gen/session/v1/session_pb` (generated TypeScript)
- Pattern: hook returns data + methods, uses Redux for state

## File Watcher (fsnotify)

**Already in use:** `github.com/fsnotify/fsnotify` is used in multiple places:
- `session/history_watcher.go` — watches `~/.claude/projects/` for new JSONL files
- `session/unfinished/watcher.go` — watches git repos
- `session/mux/autodiscover.go` — watches `/tmp/` for claude-mux sockets
- `daemon/daemon.go` and `server/auth/setup.go`

**Reuse recommendation:** Token monitoring can hook directly into the `HistoryLinker` callback or register its own `HistoryFileWatcher` on the same directory. The `NewHistoryFileWatcher(watchDir, callback)` constructor is a public API in the `session` package.

## Session Storage & JSONL Association

**How sessions link to JSONL:**
1. Active sessions: `HistoryLinker` detects via `proc_pidinfo` open files (fast path)
2. Completed sessions: `DetectByPath(projectPath)` scans `~/.claude/projects/<encoded-path>/` for most-recently-modified `.jsonl`
3. Result stored in `Instance.HistoryFilePath` (in-memory, not persisted to ent DB)

**Ent schema fields relevant to token monitoring:**
- `Session` entity has `path`, `program`, `session_type`, `created_at`, `updated_at`
- `ClaudeSession` entity (one-to-one with Session) has `claude_session_id`, `conversation_id`
- **No token count fields exist** in the ent schema — token data is not currently stored in the database

**Existing analytics infrastructure:** `server/services/analytics_store.go` implements in-memory analytics for approval decisions (tool classifications, rule triggers, etc.). This is the pattern to follow for token aggregation.

## History/JSONL Parsing Infrastructure

**Existing:** The codebase already has a full history browser at `web-app/src/app/history/page.tsx` that reads `ClaudeHistoryEntry` and `ClaudeMessage` via ConnectRPC. The `search_service.go` handles listing/searching history entries. However, **neither `ClaudeHistoryEntry` nor `ClaudeMessage` protobuf messages include token usage fields** — `ClaudeMessage` only has `role`, `content`, `timestamp`, `model`.

**Conclusion:** Token parsing is entirely new work. The JSONL file access path is solved (via `HistoryFilePath` on `Instance` or `DetectByPath`), but the Go struct to parse token fields and aggregate them does not exist yet.

## Go Module Summary

- `github.com/fsnotify/fsnotify` — already used, no new dependency
- `recharts` — new npm dep needed for dashboard charts
- No other new backend dependencies needed (standard `encoding/json`, `bufio.Scanner` for JSONL parsing)
