# Stack Research: session-history-metadata

**Date**: 2026-06-22
**Researcher**: Claude Code research agent

---

## 1. Ent ORM Schema — Structure and Patterns

### Schema layout

All schemas live in `session/ent/schema/`. Each entity is a Go struct embedding `ent.Schema` with `Fields()` and `Edges()` methods.

**Existing entities relevant to this feature:**
- `Session` (`session.go`) — the top-level entity; already has `github_pr_url` (string, optional) and `github_pr_number` (int, optional, default 0) fields
- `ClaudeSession` (`claudesession.go`) — one-to-one with Session; holds `claude_session_id`, `conversation_id`, plus settings fields; has an `edge.To("metadata", ClaudeMetadata.Type)` edge
- `ClaudeMetadata` (`claudemetadata.go`) — key/value pairs hanging off ClaudeSession

### Pattern for adding new fields

Two options match prior art:

**Option A — JSON blob field on Session** (mirrors how `github_pr_url` / `github_pr_number` were added):
```go
field.JSON("session_artifacts", []ArtifactRecord{}).
    Optional().
    Comment("Extracted artifacts from JSONL: PR links, commits, URLs."),
```
Ent supports `field.JSON()` with any Go type; it serializes to SQLite as a JSON blob. This is simplest for unstructured/heterogeneous artifact lists.

**Option B — new entity `SessionArtifact`** (mirrors `ClaudeMetadata` pattern):
- Add `session/ent/schema/sessionartifact.go` with typed fields: `artifact_type`, `url`, `pr_number`, `owner`, `repo`, `raw_text`
- Add edge `Session → SessionArtifacts` (one-to-many)
- Richer query support but requires migration + generate

**Option C — new field on ClaudeSession** for scan bookmarking:
```go
field.Int64("jsonl_scan_offset").
    Default(0).
    Comment("Byte offset up to which the JSONL file has been scanned for artifacts."),
field.Time("jsonl_scan_mod_time").
    Optional().
    Nillable().
    Comment("ModTime of JSONL file at last successful scan; used to detect changes."),
```

**CRITICAL**: The generate command requires `--feature sql/upsert` (see `session/ent/generate.go`):
```bash
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```

---

## 2. HistoryLinker — How It Triggers on JSONL Changes

### Dual detection path

`HistoryLinker` (`session/history_linker.go`) uses two complementary mechanisms:

1. **fsnotify watcher** (`HistoryFileWatcher` in `session/history_watcher.go`):
   - Watches `~/.claude/projects/` recursively via `github.com/fsnotify/fsnotify`
   - Triggers on `Create`, `Rename`, and `Write` events for `.jsonl` files (not `agent-*.jsonl`)
   - Callback calls `hl.ScanAll()` — a full rescan of all monitored instances
   - Also fires `extraCallbacks` (a registered list) — **this is the hook for the new scanner**

2. **Polling loop** (every 5 s): polls unlinked sessions via `proc_pidinfo` open-files inspection; skips already-linked sessions

### The `RegisterFileCallback` hook (key integration point)

```go
// HistoryLinker already has this method:
func (hl *HistoryLinker) RegisterFileCallback(cb func(filePath string)) {
    hl.mu.Lock()
    defer hl.mu.Unlock()
    hl.extraCallbacks = append(hl.extraCallbacks, cb)
}
```

This is exactly how `TokenStore.OnHistoryFileChanged` is wired in. The metadata scanner should use the same pattern — **no new watcher needed**.

### Integration point in server startup

`HistoryLinker.RegisterFileCallback` already exists and is called during server startup to wire in `TokenStore`. The new metadata scanner registers another callback the same way.

---

## 3. Claude JSONL Conversation File Format

### File location
`~/.claude/projects/<encoded-path>/<uuid>.jsonl`

Path encoding: every non-alphanumeric char in the project path becomes `-` (see `ClaudeProjectDirName` in `history_detector.go`).

### Line types observed

Each line is a self-contained JSON object with a `"type"` discriminator:

| `type` | Description |
|---|---|
| `"mode"` | Session mode (e.g. `"normal"`) — first line |
| `"permission-mode"` | Permission settings — second line |
| `"user"` | User turn — either human text or `tool_result` array |
| `"assistant"` | Assistant turn — `text`, `tool_use`, or `thinking` content blocks |

### Structure of `"user"` entries with `tool_result`

```json
{
  "parentUuid": "...",
  "type": "user",
  "message": {
    "role": "user",
    "content": [
      {
        "tool_use_id": "toolu_xxx",
        "type": "tool_result",
        "content": "<text output of the tool>",
        "is_error": false
      }
    ]
  },
  "toolUseResult": {
    "stdout": "<same text>",
    "stderr": "",
    "interrupted": false,
    "isImage": false
  },
  "uuid": "...",
  "timestamp": "2026-06-22T23:43:19.717Z",
  "cwd": "/home/tstapler/.stapler-squad/...",
  "sessionId": "74477bd8-...",
  "version": "2.1.186",
  "gitBranch": "ssq-pr-integration"
}
```

The `content` field of a `tool_result` block is a **plain string** (not JSON). This is the primary target for artifact extraction — it contains `git push` output, `gh pr create` output, command results, etc.

### Structure of `"assistant"` entries with `tool_use`

```json
{
  "type": "assistant",
  "message": {
    "content": [
      {
        "type": "tool_use",
        "id": "toolu_xxx",
        "name": "Bash",
        "input": {
          "command": "git push origin ...",
          "description": "..."
        }
      }
    ]
  }
}
```

### Go type definitions already exist

`session/tokens/jsonl_types.go` already defines the full struct hierarchy:
- `jsonlEntry` — outer envelope (type, role, uuid, sessionId, timestamp, message)
- `jsonlMessage` — assistant message (id, role, model, content[], usage)
- `jsonlContent` — content block (type, name, input, text, tool_use_id, content)
- `jsonlUserMessage` — user message (role, content[])
- `jsonlUserContent` — user content block (type, text, tool_use_id, content)

The new scanner can reuse or mirror these types. Note: `jsonlContent.Input` is `json.RawMessage` (to avoid large allocations for tool inputs); `jsonlContent.Text` holds assistant text.

### Fields to scan for artifact extraction

| Source field | Entry type | What to look for |
|---|---|---|
| `toolUseResult.stdout` | user/tool_result | PR URLs, commit SHAs, external URLs |
| `message.content[].content` (string) | user/tool_result | same |
| `message.content[].text` | assistant text | PR URLs, external URLs mentioned |
| `message.content[].input` (RawMessage) | assistant tool_use | git commands with URLs |

---

## 4. Regex/Parsing Patterns for Artifact Extraction

### GitHub PR URL

Already implemented in `session/repo_path.go` as `ParseGitHubURL`:
```go
prPattern := regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+)/pull/(\d+)`)
```
For scanning (not anchored to start of line):
```go
var rePRURL = regexp.MustCompile(`https?://github\.com/([^/\s]+)/([^/\s]+)/pull/(\d+)`)
```

### Commit SHAs

Git outputs 40-char hex SHAs after `commit` keyword. Pattern:
```go
var reCommitSHA = regexp.MustCompile(`(?i)\bcommit\s+([0-9a-f]{40})\b`)
// Also short SHA from `git log --oneline`:
var reShortSHA = regexp.MustCompile(`^([0-9a-f]{7,12})\s+`)
```

Note: `git push` output includes lines like:
```
   abc1234..def5678  main -> main
```
Pattern: `regexp.MustCompile(`[0-9a-f]{7,40}\.\.([0-9a-f]{7,40})\s`)` — take the right side (pushed commit).

### External URLs

```go
var reURL = regexp.MustCompile(`https?://[^\s<>"'\]\)]+`)
```
Filter out GitHub PR URLs (already captured separately) and known noise (e.g. `github.com/*/actions/`).

### Deduplication

Use a `map[string]bool` keyed on URL/SHA string. For PR URLs, also deduplicate by (owner, repo, number) tuple.

---

## 5. Go Standard Library for Incremental File Reading

### Pattern: track byte offset + modtime

The `tokens.TokenStore` uses `stat.ModTime()` to detect changes and re-parses the entire file when modified:
```go
if existing != nil && !stat.ModTime().After(existing.modTime) {
    return // cache is still valid
}
```

For the artifact scanner, which needs **incremental** reading (only new lines appended), the right pattern is:
1. Store `(byteOffset int64, modTime time.Time)` per file in the scanner's in-memory state (and persist to DB for restart survival)
2. On trigger:
   ```go
   stat, _ := os.Stat(filePath)
   if stat.ModTime().Equal(lastModTime) { return } // nothing changed
   
   f, _ := os.Open(filePath)
   f.Seek(lastByteOffset, io.SeekStart)
   scanner := bufio.NewScanner(f)
   // ... read new lines only
   lastByteOffset += bytesRead
   lastModTime = stat.ModTime()
   ```

`bufio.Scanner` with a 1 MiB buffer (matching existing code in `history.go`) handles lines up to 1 MiB each.

`io.ReadSeeker` (`os.File` implements it) + `Seek(offset, io.SeekStart)` is the standard library pattern. No external dependencies needed.

### Existing precedent in codebase

`session/history.go` already uses `bufio.NewScanner` with a 1 MiB buffer for JSONL line reading:
```go
scanner := bufio.NewScanner(file)
scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
```

And `readLastNMessagesFromFile` uses `f.ReadAt(buf, pos)` for reverse chunk reading — showing the team is comfortable with file seeking.

---

## 6. Async Background Scanning Pattern

### Existing pattern: TokenStore worker pool

`session/tokens/store.go` is the canonical example:
- A buffered `parseQueue chan string` (size 256)
- A `sync.Map` (`inflight`) to deduplicate concurrent enqueues of the same file
- `workerPoolSize = 4` goroutines reading from the queue
- fsnotify callback calls `enqueue(filePath)` — non-blocking, drops if queue full with a log warning
- `modTime`-based cache invalidation guard inside `parseAndCache`

The metadata scanner should mirror this pattern exactly:

```go
type ArtifactScanner struct {
    mu         sync.RWMutex
    offsets    map[string]*scanState // filePath → {byteOffset, modTime}
    storage    *session.Storage       // for persisting to DB
    scanQueue  chan string
    inflight   sync.Map
}

// scanState tracks incremental read position per file
type scanState struct {
    byteOffset int64
    modTime    time.Time
}

func (s *ArtifactScanner) OnHistoryFileChanged(filePath string) {
    s.enqueue(filePath)
}
```

### Integration: register with HistoryLinker

```go
// In server startup (server/server.go or equivalent):
artifactScanner := session.NewArtifactScanner(storage)
historyLinker.RegisterFileCallback(artifactScanner.OnHistoryFileChanged)
artifactScanner.Start(ctx)
```

This is identical to how `TokenStore` is wired:
```go
// Existing pattern in codebase:
tokenStore.Start(ctx)
historyLinker.RegisterFileCallback(tokenStore.OnHistoryFileChanged)
```

### Not blocking request handlers

The fsnotify callback → enqueue is O(1) and non-blocking (drops with a warning if queue full). The actual JSONL parsing happens in worker goroutines. This ensures request handlers are never blocked.

---

## 7. Storage Strategy for Artifacts

### Recommendation: JSON blob field on Session

Based on the existing schema pattern (`github_pr_url`/`github_pr_number` are direct fields, `ClaudeMetadata` is a key-value bag), the cleanest option for heterogeneous artifacts is a JSON blob:

```go
// session/ent/schema/session.go
field.JSON("session_artifacts", []*SessionArtifact{}).
    Optional().
    Comment("Structured artifacts extracted from JSONL: PRs, commits, URLs."),

field.Int64("jsonl_scan_offset").
    Default(0).
    Comment("Byte offset of last successful JSONL scan for artifact extraction."),

field.Time("jsonl_scan_mod_time").
    Optional().
    Nillable().
    Comment("ModTime at last scan; nil = never scanned."),
```

Where `SessionArtifact` is a plain Go struct (not an ent entity):
```go
type SessionArtifact struct {
    Type      string `json:"type"`       // "pr", "commit", "url"
    URL       string `json:"url"`
    Owner     string `json:"owner,omitempty"`
    Repo      string `json:"repo,omitempty"`
    PRNumber  int    `json:"pr_number,omitempty"`
    CommitSHA string `json:"commit_sha,omitempty"`
    SeenAt    string `json:"seen_at"`    // RFC3339 timestamp from JSONL entry
}
```

This avoids a new entity + edge, keeps schema diff minimal, and is compatible with the existing UpsertSession pattern.

### PR poller integration

When a PR artifact is found and `inst.GitHubPRNumber == 0`, the scanner calls:
```go
storage.UpdateInstancePRNumber(inst.Title, artifact.PRNumber)
// same method already used by PRStatusPoller
```
This feeds the existing PR status polling pipeline directly.

---

## 8. Open Question Resolutions

1. **Storage**: JSON blob on Session (see above) — simplest migration, sufficient for the use case
2. **Trigger**: Reuse `HistoryLinker.RegisterFileCallback` — no second watcher needed
3. **Deduplication**: In-memory `map[string]bool` keyed by (type + canonical URL); merged on each incremental scan pass
4. **Frontend placement**: Separate "Artifacts" tab on session detail page (parallel to "Terminal") — avoids crowding the main view
5. **HeadRef validation**: Skip — extract PR URLs from text first, let `PRStatusPoller` validate via GitHub API on its next tick

---

## Key Files

| File | Relevance |
|---|---|
| `session/history_linker.go` | `RegisterFileCallback` is the hook for wiring the scanner |
| `session/history_watcher.go` | fsnotify watcher — fires on JSONL Write events |
| `session/tokens/store.go` | Worker pool + modtime cache pattern to replicate |
| `session/tokens/jsonl_types.go` | Go struct definitions for JSONL entries |
| `session/history.go` | `conversationMessage` + `readAllMessagesFromFile` — existing parser to borrow from |
| `session/history_detector.go` | `ClaudeProjectDirName` for path encoding; `DetectByPath` |
| `session/ent/schema/session.go` | Where to add `session_artifacts`, `jsonl_scan_offset`, `jsonl_scan_mod_time` fields |
| `session/ent/schema/claudesession.go` | ClaudeSession → Session edge; holds `conversation_id` |
| `session/pr_status_poller.go` | `UpdateInstancePRNumber` / `applyPRUpdate` integration point |
| `session/repo_path.go` | `ParseGitHubURL` regex — reusable for PR URL extraction |
