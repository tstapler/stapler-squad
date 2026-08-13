# Implementation Plan: session-history-metadata

**Feature**: Async JSONL artifact extraction — surface PR links, commits, and URLs from session history in a new "Artifacts" tab
**Date**: 2026-06-22
**Status**: Ready for implementation
**ADRs**: ADR-010-session-artifacts-json-blob-storage.md

---

## Dependency Visualization

```
Phase 1: Schema + Data Layer
  Task 1.1.1a: ent schema field        ──► Task 1.1.1b: ent generate
  Task 1.1.2a: proto SessionArtifacts  ──► Task 1.1.2b: make generate-proto
  Task 1.1.3a: storage method

Phase 2: Extractor Engine (depends on Phase 1)
  Task 2.1.1a: ArtifactBlob types      ──► Task 2.1.1b: regex extractors
  Task 2.1.2a: ArtifactExtractor struct ──► Task 2.1.2b: incremental scan loop
  Task 2.1.3a: storage persistence + PR poller integration

Phase 3: Wiring (depends on Phase 2)
  Task 3.1.1a: dependencies.go wiring   ──► Task 3.1.1b: startup backfill walk
  Task 3.1.2a: Instance field + event bus push

Phase 4: Frontend (depends on Phase 1 proto; can start after Task 1.1.2b)
  Task 4.1.1a: ArtifactsTab component
  Task 4.1.1b: ArtifactsTab.css.ts
  Task 4.1.2a: SessionDetailTab union + SessionDetailView wiring

Phase 5: Tests (depends on Phase 2 + Phase 4)
  Task 5.1.1a: Go extractor unit tests
  Task 5.1.2a: Frontend component tests
  Task 5.1.3a: e2e smoke test
```

---

## Phase 1: Schema and Data Layer

### Epic 1.1: Persistence Foundations
**Goal**: Add the `session_artifacts` JSON blob field to the ent schema, regenerate the ORM, and add the `SessionArtifacts` proto message — establishing the two storage contracts everything else builds on.

#### Story 1.1.1: Ent schema field
**As a** backend service, **I want** a `session_artifacts` TEXT field on the Session ent entity, **so that** extracted artifact data survives process restarts.

**Acceptance Criteria**:
- `session_artifacts` field exists in ent schema as `Optional()` string with empty default
- `make build` succeeds after `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` (CRITICAL: must include `--feature sql/upsert`)
- SQLite migration applies cleanly (nullable TEXT column, no data loss)

**Files**:
- `session/ent/schema/session.go`
- `session/ent/generate.go` (verify command present)
- All auto-generated files under `session/ent/` (committed together)

##### Task 1.1.1a: Add field to ent schema (~3 min)
- Open `session/ent/schema/session.go`
- In `Fields()`, append after `github_pr_number`:
  ```go
  field.String("session_artifacts").
      Optional().
      Default("").
      Comment("JSON-encoded SessionArtifactsBlob: PRURLs, CommitSHAs, ExternalURLs, scan offset."),
  ```
- Files: `session/ent/schema/session.go`

##### Task 1.1.1b: Regenerate ent ORM (~2 min)
- Run: `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
- Run: `go build ./...` to verify no compilation errors
- Stage all changed files under `session/ent/` — they must be committed together
- Files: `session/ent/` (all auto-generated — do not edit manually)

---

#### Story 1.1.2: Proto SessionArtifacts message
**As a** frontend client, **I want** a `SessionArtifacts` proto message embedded in the `Session` message, **so that** artifact data flows through the existing WatchSessions event stream without a new RPC.

**Acceptance Criteria**:
- `SessionArtifacts` message with repeated `pr_urls`, `commit_shas`, `external_urls` fields and `last_scanned_at` timestamp exists in `types.proto`
- `session.artifacts` field added to `Session` message at field number 70
- `make generate-proto` succeeds; TypeScript bindings regenerated in `web-app/src/gen/`

**Files**:
- `proto/session/v1/types.proto`
- `session/gen/session/v1/` (auto-generated Go bindings)
- `web-app/src/gen/session/v1/` (auto-generated TS bindings)

##### Task 1.1.2a: Add proto message and field (~3 min)
- Open `proto/session/v1/types.proto`
- After the `SessionGoalSummary` message block (around line 247), add:
  ```protobuf
  // SessionArtifacts holds structured artifacts extracted from the session's
  // Claude Code JSONL conversation history.
  message SessionArtifacts {
    // GitHub PR URLs found in tool_result output (e.g. from gh pr create).
    repeated string pr_urls = 1;
    // Git commit SHAs (40-char) found in tool_result output.
    repeated string commit_shas = 2;
    // External URLs found in tool_result output (capped at 50 entries).
    repeated string external_urls = 3;
    // When the JSONL file was last successfully scanned.
    google.protobuf.Timestamp last_scanned_at = 4;
  }
  ```
- In the `Session` message, append at the end (after field 69, `detected_context`):
  ```protobuf
  // Structured artifacts extracted from the session's JSONL conversation history.
  // Populated asynchronously by ArtifactExtractor; nil until first scan completes.
  SessionArtifacts artifacts = 70;
  ```
- Files: `proto/session/v1/types.proto`

##### Task 1.1.2b: Regenerate proto bindings (~2 min)
- Run: `make generate-proto`
- Verify `session/gen/session/v1/types.pb.go` and `web-app/src/gen/session/v1/types_pb.ts` both contain `SessionArtifacts`
- Files: `session/gen/session/v1/`, `web-app/src/gen/session/v1/`

---

#### Story 1.1.3: Storage persistence method
**As a** Go backend, **I want** an `UpdateInstanceArtifacts(title string, blob string) error` method on Storage, **so that** the extractor can atomically persist the JSON blob after each scan.

**Acceptance Criteria**:
- `UpdateInstanceArtifacts` exists on both the `Storage` concrete type and the `StorageInterface` (if present)
- Updates only `session_artifacts` column — no other fields touched
- Returns a wrapped error on DB failure; nil on success

**Files**:
- `session/storage.go`

##### Task 1.1.3a: Add UpdateInstanceArtifacts and GetInstanceArtifacts to Storage (~4 min)
- Open `session/storage.go`
- Locate `UpdateInstancePRNumber` (around line 455) to use as a pattern
- Add immediately after it:
  ```go
  // UpdateInstanceArtifacts persists the JSON-encoded artifact blob for a session.
  // Only the session_artifacts column is touched; all other fields are unchanged.
  func (s *Storage) UpdateInstanceArtifacts(title string, blob string) error {
      return s.entClient.Session.
          Update().
          Where(session_ent.Title(title)).
          SetSessionArtifacts(blob).
          Exec(context.Background())
  }

  // GetInstanceArtifacts loads the raw JSON-encoded artifact blob for a session.
  // Returns ("", nil) if the session exists but has no artifacts yet.
  func (s *Storage) GetInstanceArtifacts(title string) (string, error) {
      sess, err := s.entClient.Session.
          Query().
          Where(session_ent.Title(title)).
          Select(session_ent.FieldSessionArtifacts).
          Only(context.Background())
      if err != nil {
          return "", fmt.Errorf("GetInstanceArtifacts: %w", err)
      }
      return sess.SessionArtifacts, nil
  }
  ```
- If a `StorageInterface` interface exists in the file, add matching method signatures there as well
- Files: `session/storage.go`

---

## Phase 2: Extractor Engine

### Epic 2.1: ArtifactExtractor Service
**Goal**: Implement the `ArtifactExtractor` struct that mirrors the `TokenStore` worker-pool pattern — enqueuing JSONL files, scanning only new bytes (via byte offset), extracting artifacts from `tool_result` blocks only, and persisting the result blob.

#### Story 2.1.0: JSONL type definitions (private copy) — ADR-010 decision
**Decision**: define a minimal private copy of the JSONL envelope structs in `session/artifacts/jsonl.go` rather than exporting `tokens.jsonlEntry` etc. Rationale: the `tokens` package's `jsonlUserContent` deliberately omits the `Input` field to avoid 237 KB allocations per call; exporting and sharing these structs would create cross-package coupling that could silently regress that optimization. A private copy in the `artifacts` package is isolated and can include only the fields the scanner needs.

##### Task 2.1.0a: Define private JSONL struct copies (~3 min)
- Create `session/artifacts/jsonl.go`:
  ```go
  package artifacts

  import "encoding/json"

  // artifactEntry is a minimal copy of the JSONL envelope fields the scanner needs.
  // Deliberately does NOT embed session/tokens types to avoid coupling to that package's
  // performance-optimized omissions.
  type artifactEntry struct {
      Type    string          `json:"type"`
      Message json.RawMessage `json:"message"`
  }

  type artifactMessage struct {
      Role    string                 `json:"role"`
      Content []artifactContentBlock `json:"content"`
  }

  type artifactContentBlock struct {
      Type       string          `json:"type"`
      ToolUseID  string          `json:"tool_use_id"`
      Content    json.RawMessage `json:"content"` // string or []textBlock (tool_result)
      // tool_use fields:
      Name       string          `json:"name"`    // "Bash", "Write", "Edit", etc.
      Input      json.RawMessage `json:"input"`   // {"command":"...", "description":"..."} for Bash
  }

  // bashInput holds the Bash tool_use input fields we care about.
  type bashInput struct {
      Command string `json:"command"`
  }
  ```
- Files: `session/artifacts/jsonl.go`

---

#### Story 2.1.1: Artifact types and regex extractors
**As a** developer, **I want** typed Go structs and compiled regexes for the artifact blob and extraction logic, **so that** the extractor has well-defined data contracts and efficient pattern matching.

**Acceptance Criteria**:
- `SessionArtifactsBlob` struct with JSON tags matches the proto field names
- `ScanOffsetBytes int64` and `LastScannedAt time.Time` included for incremental scanning
- Regex patterns compile at init time (package-level `var`); no per-call `regexp.MustCompile`
- `ExtractFromToolResult(text string) (prURLs, commitSHAs, externalURLs []string)` uses map-based dedup (not substring matching)
- **NEW**: `ExtractFromToolUseCommand(command string)` extracts structured signals from bash command strings — `gh pr create` args (title, body), `gh pr merge N`, `git commit -m "..."` (commit message), `go get`/`npm install` package references. This gives earlier/higher-confidence signal than waiting for the matching `tool_result` output.

**Command extraction targets** (extracted from `assistant` entries where `content[].type == "tool_use"` and `content[].name == "Bash"`):

| Pattern | Example | Extracted |
|---|---|---|
| `gh pr create` | `gh pr create --title "feat: foo" --body "..."` | PR title, body preview |
| `gh pr merge N` | `gh pr merge 42 --squash` | PR number being merged |
| `git commit -m` | `git commit -m "feat: add search"` | commit message text |
| `go get pkg@ver` | `go get github.com/foo/bar@v1.2.3` | package reference |
| `npm install pkg` | `npm install react-query` | package reference |

Note: command extraction supplements (does not replace) `tool_result` extraction. A `gh pr create` command gives you the title; the matching `tool_result` gives you the created PR URL. Both should be stored.

**Files**:
- `session/artifacts/types.go` (new file)
- `session/artifacts/extractor.go` (new file)

##### Task 2.1.1a: Define SessionArtifactsBlob and regex vars (~4 min)
- Create `session/artifacts/types.go`:
  ```go
  package artifacts

  import "time"

  // SessionArtifactsBlob is the JSON-serialized payload stored in session_artifacts TEXT column.
  type SessionArtifactsBlob struct {
      PRURLs          []string      `json:"pr_urls"`
      CommitSHAs      []string      `json:"commit_shas"`
      ExternalURLs    []string      `json:"external_urls"`
      Commands        []CommandArtifact `json:"commands,omitempty"` // from tool_use bash invocations
      ScanOffsetBytes int64         `json:"scan_offset_bytes"`
      LastScannedAt   time.Time     `json:"last_scanned_at"`
  }

  // CommandArtifact records a structured signal extracted from a tool_use bash command.
  type CommandArtifact struct {
      Type    string `json:"type"`    // "gh_pr_create", "gh_pr_merge", "git_commit", "package_install"
      Command string `json:"command"` // first 200 chars of the raw command for display
      Detail  string `json:"detail"`  // extracted value: PR title, commit message, pkg name, PR number
  }

  const maxExternalURLs = 50
  const maxCommands    = 30
  ```
- Files: `session/artifacts/types.go`

##### Task 2.1.1b: Implement regex extractors (~5 min)
- Create `session/artifacts/extractor.go` with package-level compiled regexes:
  ```go
  package artifacts

  import (
      "regexp"
  )

  var (
      // GitHub PR URL: https://github.com/<owner>/<repo>/pull/<number>
      rePRURL = regexp.MustCompile(`https://github\.com/[\w.-]+/[\w.-]+/pull/\d+`)
      // 40-hex commit SHA (standalone word boundary)
      reCommitSHA = regexp.MustCompile(`\b[0-9a-f]{40}\b`)
      // General https:// URL
      reExternalURL = regexp.MustCompile(`https?://[^\s"'<>]+`)
  )

  // ExtractFromToolResult extracts artifacts from the text content of a single
  // tool_result block. Callers must pass only tool_result content — never raw
  // assistant text — to avoid false positives.
  func ExtractFromToolResult(text string) (prURLs, commitSHAs, externalURLs []string) {
      prSet := make(map[string]struct{})
      for _, m := range rePRURL.FindAllString(text, -1) {
          if _, seen := prSet[m]; !seen {
              prSet[m] = struct{}{}
              prURLs = append(prURLs, m)
          }
      }
      shaSet := make(map[string]struct{})
      for _, m := range reCommitSHA.FindAllString(text, -1) {
          if _, seen := shaSet[m]; !seen {
              shaSet[m] = struct{}{}
              commitSHAs = append(commitSHAs, m)
          }
      }
      for _, m := range reExternalURL.FindAllString(text, -1) {
          // Skip URLs already captured as PR URLs (exact key match, not substring)
          if _, isPR := prSet[m]; !isPR {
              externalURLs = append(externalURLs, m)
          }
      }
      return
  }

  var (
      reGHPRCreate   = regexp.MustCompile(`gh pr create.*?--title\s+"([^"]+)"`)
      reGHPRMerge    = regexp.MustCompile(`gh pr merge\s+(\d+)`)
      reGitCommitMsg = regexp.MustCompile(`git commit.*?-m\s+"([^"]+)"`)
      reGoGet        = regexp.MustCompile(`go get\s+([\w./-]+@[\w./-]+)`)
      reNPMInstall   = regexp.MustCompile(`npm install\s+([\w@./-]+)`)
  )

  // ExtractFromBashCommand extracts a structured CommandArtifact from a Bash tool_use
  // input command. Returns nil if no known pattern matches.
  // Only call this on tool_use inputs (not tool_result outputs) to avoid double-counting.
  func ExtractFromBashCommand(command string) *CommandArtifact {
      // gh pr create
      if m := reGHPRCreate.FindStringSubmatch(command); m != nil {
          return &CommandArtifact{Type: "gh_pr_create", Command: truncate(command, 200), Detail: m[1]}
      }
      // gh pr merge N
      if m := reGHPRMerge.FindStringSubmatch(command); m != nil {
          return &CommandArtifact{Type: "gh_pr_merge", Command: truncate(command, 200), Detail: m[1]}
      }
      // git commit -m "..."
      if m := reGitCommitMsg.FindStringSubmatch(command); m != nil {
          return &CommandArtifact{Type: "git_commit", Command: truncate(command, 200), Detail: m[1]}
      }
      // go get pkg@ver
      if m := reGoGet.FindStringSubmatch(command); m != nil {
          return &CommandArtifact{Type: "package_install", Command: truncate(command, 200), Detail: m[1]}
      }
      // npm install pkg
      if m := reNPMInstall.FindStringSubmatch(command); m != nil {
          return &CommandArtifact{Type: "package_install", Command: truncate(command, 200), Detail: m[1]}
      }
      return nil
  }

  func truncate(s string, n int) string {
      if len(s) > n {
          return s[:n] + "…"
      }
      return s
  }
  ```
- Files: `session/artifacts/extractor.go`

---

#### Story 2.1.2: ArtifactExtractor struct and scan loop
**As a** system component, **I want** an `ArtifactExtractor` that receives JSONL file change callbacks and scans only new bytes per file, **so that** extraction is O(new bytes) not O(total file size) on each update.

**Acceptance Criteria**:
- `ArtifactExtractor` mirrors `TokenStore` worker pool (buffered chan, 4 goroutines, `sync.Map` in-flight dedup)
- `OnHistoryFileChanged(filePath string)` skips `agent-` prefix files and non-`.jsonl` files
- Per-file scan reads from `ScanOffsetBytes` via `f.Seek(offset, io.SeekStart)` — never re-reads old bytes
- JSON parse errors on partial last lines are silently skipped (not logged as corruption)
- Only `type == "user"` entries with `content[].type == "tool_result"` are processed; assistant text is ignored
- `scanner.Err()` is checked after the scan loop; I/O errors abort without advancing the offset
- Scanner buffer is 10 MB (matching `tokens/parser.go`) to handle large base64 tool outputs
- `OnScanComplete` hook is an injected `func(title string, blob *SessionArtifactsBlob)` callback (default no-op), NOT embedded in a closure — makes the extractor testable without a live event bus
- `readFn func(title string) (string, error)` is injected alongside `storeFn` to load existing blob for merge
- On `Start()`, call `readFn` for each session to seed in-memory `offsets` from `blob.ScanOffsetBytes`

**Files**:
- `session/artifacts/store.go` (new file)

##### Task 2.1.2a: ArtifactExtractor struct definition and constructor (~4 min)
- Create `session/artifacts/store.go`:
  ```go
  package artifacts

  import (
      "context"
      "strings"
      "sync"
      "path/filepath"

      "github.com/tstapler/stapler-squad/log"
  )

  const (
      artifactWorkerPoolSize = 4
      artifactQueueSize      = 256
      maxScannerTokenSize    = 10 * 1024 * 1024 // 10 MB, matches tokens/parser.go
  )

  // ArtifactExtractor reads Claude Code JSONL files and extracts structured
  // artifacts (PR URLs, commit SHAs, external URLs) using an incremental
  // byte-offset scan. Mirrors the TokenStore worker pool pattern.
  type ArtifactExtractor struct {
      queue    chan string
      inflight sync.Map // key: filePath, value: struct{}

      // offsets tracks the last scanned byte offset per file.
      offsetsMu sync.Mutex
      offsets   map[string]int64

      // storeFn is called with (sessionTitle, jsonBlob) after each successful scan.
      storeFn func(title, blob string) error
      // readFn loads the existing stored blob for a session (for merge on scan).
      readFn func(title string) (string, error)
      // lookupTitle maps a JSONL file path to its session title.
      lookupTitle func(filePath string) (string, bool)
      // OnScanComplete is called after a successful scan with new artifacts.
      // Inject event-bus publish logic here; defaults to no-op.
      // This keeps ArtifactExtractor testable without a live event bus.
      OnScanComplete func(title string, blob *SessionArtifactsBlob)

      cancelFunc context.CancelFunc
  }

  // NewArtifactExtractor creates an ArtifactExtractor.
  // storeFn: persists the JSON blob (wraps storage.UpdateInstanceArtifacts).
  // readFn: loads the existing blob (wraps storage.GetInstanceArtifacts).
  // lookupTitle: resolves a JSONL path to its session title.
  func NewArtifactExtractor(
      storeFn func(title, blob string) error,
      readFn func(title string) (string, error),
      lookupTitle func(filePath string) (string, bool),
  ) *ArtifactExtractor {
      return &ArtifactExtractor{
          queue:          make(chan string, artifactQueueSize),
          offsets:        make(map[string]int64),
          storeFn:        storeFn,
          readFn:         readFn,
          lookupTitle:    lookupTitle,
          OnScanComplete: func(_ string, _ *SessionArtifactsBlob) {}, // no-op default
      }
  }
  ```
- Files: `session/artifacts/store.go`

##### Task 2.1.2b: Start/Stop, enqueue, worker, and scan loop (~5 min)
- Continue `session/artifacts/store.go` — add:
  ```go
  // Start launches worker goroutines and the startup backfill walk.
  func (ae *ArtifactExtractor) Start(ctx context.Context, historyDir string) {
      ctx, cancel := context.WithCancel(ctx)
      ae.cancelFunc = cancel

      for i := 0; i < artifactWorkerPoolSize; i++ {
          go ae.worker(ctx)
      }
      go ae.walkAndEnqueue(ctx, historyDir)
  }

  // Stop cancels background goroutines.
  func (ae *ArtifactExtractor) Stop() {
      if ae.cancelFunc != nil {
          ae.cancelFunc()
      }
  }

  // OnHistoryFileChanged is the HistoryLinker callback — filters and enqueues.
  func (ae *ArtifactExtractor) OnHistoryFileChanged(filePath string) {
      if !strings.HasSuffix(filePath, ".jsonl") {
          return
      }
      if strings.HasPrefix(filepath.Base(filePath), "agent-") {
          return
      }
      ae.enqueue(filePath)
  }

  func (ae *ArtifactExtractor) enqueue(filePath string) {
      if _, loaded := ae.inflight.LoadOrStore(filePath, struct{}{}); loaded {
          return
      }
      select {
      case ae.queue <- filePath:
      default:
          ae.inflight.Delete(filePath)
          log.Warn("[ArtifactExtractor] queue full, dropping", "path", filePath)
      }
  }

  func (ae *ArtifactExtractor) worker(ctx context.Context) {
      for {
          select {
          case <-ctx.Done():
              return
          case filePath := <-ae.queue:
              ae.scanFile(filePath)
          }
      }
  }
  ```
- Files: `session/artifacts/store.go`

---

#### Story 2.1.3: Scan logic, persistence, and PR poller integration
**As a** session operator, **I want** artifact extraction to feed PR numbers back into the PR status poller, **so that** sessions that open a PR mid-run automatically gain PR polling without manual input.

**Acceptance Criteria**:
- `scanFile` reads ONLY new bytes from `ScanOffsetBytes` using `f.Seek`
- Partial last line (JSON unmarshal error) is silently skipped, offset not advanced past it
- Deduplication: PRURLs and CommitSHAs are deduplicated; ExternalURLs capped at 50
- After scan, existing blob is loaded, merged with new findings, then stored via `storeFn`
- If a new PR URL is found and `lookupTitle` resolves to an active session, `UpdateInstancePRNumber` is called (race guard: only if `inst.GitHubPRNumber == 0`)

**Files**:
- `session/artifacts/store.go` (continued)
- `session/artifacts/scan.go` (new file for scanFile logic — keeps store.go readable)

##### Task 2.1.3a: Implement scanFile and blob merge (~5 min)
- Create `session/artifacts/scan.go`:
  ```go
  package artifacts

  import (
      "bufio"
      "encoding/json"
      "io"
      "os"
      "time"

      "github.com/tstapler/stapler-squad/log"
      "github.com/tstapler/stapler-squad/session/tokens"
  )

  func (ae *ArtifactExtractor) scanFile(filePath string) {
      defer ae.inflight.Delete(filePath)

      f, err := os.Open(filePath)
      if err != nil {
          return // file may have been deleted
      }
      defer f.Close()

      ae.offsetsMu.Lock()
      offset := ae.offsets[filePath]
      ae.offsetsMu.Unlock()

      if offset > 0 {
          if _, err := f.Seek(offset, io.SeekStart); err != nil {
              log.Warn("[ArtifactExtractor] seek failed", "path", filePath, "err", err)
              return
          }
      }

      var newPRURLs, newCommitSHAs, newExternalURLs []string
      var newCommands []CommandArtifact
      scanner := bufio.NewScanner(f)
      scanner.Buffer(make([]byte, maxScannerTokenSize), maxScannerTokenSize) // 10 MB, matches tokens/parser.go

      for scanner.Scan() {
          line := scanner.Bytes()

          var entry artifactEntry // uses private copy in session/artifacts/jsonl.go
          if err := json.Unmarshal(line, &entry); err != nil {
              // Partial last line — stop here; do NOT advance offset past it
              break
          }

          switch entry.Type {
          case "user":
              // Extract from tool_result content (actual command output)
              var msg artifactMessage
              if err := json.Unmarshal(entry.Message, &msg); err != nil {
                  continue
              }
              for _, c := range msg.Content {
                  if c.Type != "tool_result" {
                      continue
                  }
                  text := extractToolResultText(c.Content)
                  prs, shas, urls := ExtractFromToolResult(text)
                  newPRURLs = append(newPRURLs, prs...)
                  newCommitSHAs = append(newCommitSHAs, shas...)
                  newExternalURLs = append(newExternalURLs, urls...)
              }
          case "assistant":
              // Extract from tool_use input (bash commands — gives earlier signal than output)
              var msg artifactMessage
              if err := json.Unmarshal(entry.Message, &msg); err != nil {
                  continue
              }
              for _, c := range msg.Content {
                  if c.Type != "tool_use" || c.Name != "Bash" {
                      continue
                  }
                  var inp bashInput
                  if err := json.Unmarshal(c.Input, &inp); err != nil || inp.Command == "" {
                      continue
                  }
                  if cmd := ExtractFromBashCommand(inp.Command); cmd != nil {
                      newCommands = append(newCommands, *cmd)
                  }
              }
          }
      }

      // Check for scanner errors (e.g. token-too-long) — abort without advancing offset
      if err := scanner.Err(); err != nil {
          log.Warn("[ArtifactExtractor] scanner error", "path", filePath, "err", err)
          return
      }

      // Use file position as the authoritative new offset (avoids \r\n miscounting)
      newOffset, _ := f.Seek(0, io.SeekCurrent)
      bytesRead := newOffset - int64(offset)
      if bytesRead <= 0 {
          return // no new content
      }

      ae.offsetsMu.Lock()
      ae.offsets[filePath] = newOffset
      ae.offsetsMu.Unlock()

      if len(newPRURLs)+len(newCommitSHAs)+len(newExternalURLs)+len(newCommands) == 0 {
          return
      }

      title, ok := ae.lookupTitle(filePath)
      if !ok {
          return
      }

      blob := ae.mergeAndPersist(title, newOffset, newPRURLs, newCommitSHAs, newExternalURLs, newCommands)
      if err := ae.storeFn(title, blob); err != nil {
          log.Warn("[ArtifactExtractor] persist failed", "session", title, "err", err)
          return
      }
      ae.OnScanComplete(title, blob) // publishes EventSessionUpdated — injected from dependencies.go
  }

  // extractToolResultText converts tool_result content (string or []textBlock) to plain text.
  func extractToolResultText(raw json.RawMessage) string {
      // tool_result content can be a plain string or [{type:"text", text:"..."}]
      var text string
      if err := json.Unmarshal(raw, &text); err == nil {
          return text
      }
      var blocks []struct {
          Type string `json:"type"`
          Text string `json:"text"`
      }
      if err := json.Unmarshal(raw, &blocks); err != nil {
          return ""
      }
      var sb strings.Builder
      for _, b := range blocks {
          if b.Type == "text" {
              sb.WriteString(b.Text)
              sb.WriteByte('\n')
          }
      }
      return sb.String()
  }
  ```

  Note: uses private `artifactEntry`/`artifactMessage`/`artifactContentBlock` types from `session/artifacts/jsonl.go` (Task 2.1.0a). Does NOT import `session/tokens` types.

- Files: `session/artifacts/scan.go`

##### Task 2.1.3b: mergeAndPersist, walkAndEnqueue, and startup offset restore (~5 min)
- Add to `session/artifacts/store.go`:
  ```go
  // mergeAndPersist LOADS the existing stored blob via readFn, merges new findings
  // with existing artifacts, enforces dedup and caps, then returns the JSON-encoded
  // merged blob. This ensures prior scan results are never lost on overwrite.
  func (ae *ArtifactExtractor) mergeAndPersist(
      title string,
      newOffset int64,
      newPRURLs, newCommitSHAs, newExternalURLs []string,
  ) *SessionArtifactsBlob {
      // Load existing blob; ignore read errors (treat as empty for new sessions)
      existing := &SessionArtifactsBlob{}
      if raw, err := ae.readFn(title); err == nil && raw != "" {
          _ = json.Unmarshal([]byte(raw), existing)
      }
      blob := &SessionArtifactsBlob{
          PRURLs:          dedup(append(existing.PRURLs, newPRURLs...)),
          CommitSHAs:      dedup(append(existing.CommitSHAs, newCommitSHAs...)),
          ExternalURLs:    cap50(dedup(append(existing.ExternalURLs, newExternalURLs...))),
          ScanOffsetBytes: newOffset,
          LastScannedAt:   time.Now().UTC(),
      }
      return blob
  }

  // SeedOffsets loads each session's existing blob at startup to restore
  // the byte offset — so the first scan after restart reads only new bytes.
  // Takes []*session.Instance directly (avoids a separate private snapshot type).
  // Call before Start().
  func (ae *ArtifactExtractor) SeedOffsets(instances []*session.Instance) {
      ae.offsetsMu.Lock()
      defer ae.offsetsMu.Unlock()
      for _, inst := range instances {
          fp := inst.HistoryFilePath() // uses exported accessor
          if fp == "" {
              continue
          }
          raw, err := ae.readFn(inst.Title)
          if err != nil || raw == "" {
              continue
          }
          var blob SessionArtifactsBlob
          if err := json.Unmarshal([]byte(raw), &blob); err == nil && blob.ScanOffsetBytes > 0 {
              ae.offsets[fp] = blob.ScanOffsetBytes
          }
      }
  }
  ```

  // walkAndEnqueue mirrors TokenStore.walkAndEnqueue — walks historyDir on startup.
  func (ae *ArtifactExtractor) walkAndEnqueue(ctx context.Context, historyDir string) {
      if historyDir == "" {
          return
      }
      err := filepath.WalkDir(historyDir, func(path string, d fs.DirEntry, err error) error {
          if err != nil || d.IsDir() {
              return nil
          }
          if strings.HasSuffix(path, ".jsonl") && !strings.HasPrefix(d.Name(), "agent-") {
              ae.enqueue(path)
          }
          select {
          case <-ctx.Done():
              return filepath.SkipAll
          default:
              return nil
          }
      })
      if err != nil {
          log.Warn("[ArtifactExtractor] walkAndEnqueue error", "dir", historyDir, "err", err)
      }
  }

  func dedup(ss []string) []string { /* set dedup, preserve order */ }
  func cap50(ss []string) []string {
      if len(ss) > maxExternalURLs {
          return ss[:maxExternalURLs]
      }
      return ss
  }
  ```
- Files: `session/artifacts/store.go`

---

## Phase 3: Wiring

### Epic 3.1: Backend Integration
**Goal**: Wire `ArtifactExtractor` into `server/dependencies.go` alongside `TokenStore`, populate `Instance.Artifacts` on reads, and push `EventSessionUpdated` when artifacts change so WatchSessions clients receive live updates.

#### Story 3.1.1: Wire ArtifactExtractor into dependencies
**As a** running server, **I want** ArtifactExtractor started at boot and connected to HistoryLinker callbacks, **so that** artifact extraction begins immediately on startup without user action.

**Acceptance Criteria**:
- `ArtifactExtractor` is constructed after `tokenStore` using the same `historyDir`
- `historyLinker.RegisterFileCallback(extractor.OnHistoryFileChanged)` called on line after `tokenStore` registration (line ~788)
- `extractor.Start(context.Background(), historyDir)` called
- `lookupTitle` closure iterates storage instances to find matching `HistoryFilePath`

**Files**:
- `server/dependencies.go`

##### Task 3.1.1a: Add `HistoryFilePath()` accessor and `FindInstanceByHistoryPath` helper (~3 min)
- Open `session/instance.go` and add a safe accessor so external packages never touch `stateMutex` directly:
  ```go
  // HistoryFilePath returns the JSONL history file path for the session.
  // Safe for use from any goroutine; acquires stateMutex internally.
  func (i *Instance) HistoryFilePath() string {
      i.stateMutex.RLock()
      defer i.stateMutex.RUnlock()
      return i.historyFilePath // use the actual field name found in instance.go
  }
  ```
  (If `HistoryFilePath` is already a public field, skip the accessor and document that it must only be read under `stateMutex.RLock` — add a comment next to the field.)
- Add a package-level helper in `session/artifact_lookup.go` (new tiny file):
  ```go
  package session

  // FindInstanceByHistoryPath returns the title of the session whose JSONL
  // history file matches filePath. Returns ("", false) if not found.
  // Safe to call concurrently — uses HistoryFilePath() accessor per instance.
  func FindInstanceByHistoryPath(instances []*Instance, filePath string) (string, bool) {
      for _, inst := range instances {
          if inst.HistoryFilePath() == filePath {
              return inst.Title, true
          }
      }
      return "", false
  }
  ```
- Files: `session/instance.go`, `session/artifact_lookup.go`

##### Task 3.1.1b: Wire ArtifactExtractor in dependencies.go (~4 min)
- Open `server/dependencies.go`
- Find the block starting at line ~785 (`historyLinker.RegisterFileCallback(tokenStore.OnHistoryFileChanged)`)
- After `tokenStore.Start(context.Background())`, add (NOTE: uses `instances` slice already in scope in `BuildDependencies`, NOT `storage.GetInstances()` which does not exist):
  ```go
  artifactExtractor := artifacts.NewArtifactExtractor(
      func(title, blob string) error {
          return storage.UpdateInstanceArtifacts(title, blob)
      },
      func(title string) (string, error) {
          return storage.GetInstanceArtifacts(title) // new method added in storage.go (Task 1.1.3b)
      },
      func(filePath string) (string, bool) {
          // Use live snapshot so sessions created after startup are included.
          return session.FindInstanceByHistoryPath(snapshotFn(), filePath)
      },
  )
  artifactExtractor.OnScanComplete = func(title string, blob *artifacts.SessionArtifactsBlob) {
      // Update in-memory Instance and publish event bus notification
      // (logic lives here, not inside ArtifactExtractor, so the extractor stays testable)
      for _, inst := range instances {
          if inst.Title == title {
              inst.SetArtifacts(blob) // new method on Instance (Task 3.1.2a)
              eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"artifacts"}))
              break
          }
      }
  }
  historyLinker.RegisterFileCallback(artifactExtractor.OnHistoryFileChanged)
  // Seed in-memory offsets from DB so restart doesn't re-scan from byte 0.
  // Pass a live-snapshot func so sessions created after startup are included.
  snapshotFn := func() []*session.Instance { return sessionManager.Instances() }
  artifactExtractor.SeedOffsets(snapshotFn())
  artifactExtractor.Start(context.Background(), historyDir)
  log.Info("ArtifactExtractor initialized", "historyDir", historyDir)
  ```
- Add imports: `"github.com/tstapler/stapler-squad/session/artifacts"`, `"github.com/tstapler/stapler-squad/session"`
- Files: `server/dependencies.go`

##### Task 3.1.1c: PR poller integration via OnScanComplete (~3 min)
- PR poller integration is handled in the `OnScanComplete` closure in `dependencies.go` (Task 3.1.1b), not inside the extractor itself. This avoids cross-package lock access.
- In the `OnScanComplete` closure, after setting `inst.Artifacts`:
  ```go
  // Feed PR status poller if we found PR URLs and poller doesn't have one yet
  for _, prURL := range blob.PRURLs {
      if inst.HasGitHubPR() { // new exported method on Instance — acquires stateMutex internally
          break
      }
      if ref, err := session.ParseGitHubURL(prURL); err == nil && ref.PRNumber > 0 {
          if err := storage.UpdateInstancePRNumber(inst.Title, ref.PRNumber); err != nil {
              log.Warn("ArtifactExtractor: failed to update PR number", "session", inst.Title, "err", err)
          }
          break // one PR URL is enough
      }
  }
  ```
- Add `HasGitHubPR() bool` method to `session/instance.go` that acquires `stateMutex.RLock` and returns `i.GitHubPRNumber > 0`
- Files: `server/dependencies.go`, `session/instance.go`

---

#### Story 3.1.2: Instance field and event push
**As a** frontend client, **I want** the WatchSessions stream to emit artifact updates, **so that** the Artifacts tab shows new PR links within seconds of them appearing in the JSONL file.

**Acceptance Criteria**:
- `Instance` has an `Artifacts *artifacts.SessionArtifactsBlob` field (in-memory, loaded from DB on restore)
- `toProtoSession()` (or equivalent) maps `Artifacts` → `SessionArtifacts` proto message
- `storeFn` also sets `inst.Artifacts` in memory and calls `eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"artifacts"}))`

**Files**:
- `session/instance.go` (add Artifacts field)
- `server/services/event_converter.go` (map Artifacts → proto)
- `session/storage.go` (load artifacts blob on restore)

##### Task 3.1.2a: Add Artifacts to Instance and map to proto (~5 min)
- Open `session/instance.go`; add field `Artifacts *artifacts_pkg.SessionArtifactsBlob` (import aliased to avoid `artifacts` name collision)
- In `session/storage.go` `LoadInstances()` / row-to-instance conversion: unmarshal `session_artifacts` JSON into `inst.Artifacts` if non-empty
- Open `server/services/event_converter.go` (contains `toProtoSession` or equivalent); in the Session→proto mapping, add:
  ```go
  if inst.Artifacts != nil {
      protoSess.Artifacts = &sessionv1.SessionArtifacts{
          PrUrls:      inst.Artifacts.PRURLs,
          CommitShas:  inst.Artifacts.CommitSHAs,
          ExternalUrls: inst.Artifacts.ExternalURLs,
          LastScannedAt: timestamppb.New(inst.Artifacts.LastScannedAt),
      }
  }
  ```
- In `storeFn` (in `dependencies.go` closure or a helper method on ArtifactExtractor): after DB write succeeds, look up Instance in storage, set `inst.Artifacts`, publish `NewSessionUpdatedEvent(inst, []string{"artifacts"})`
- Files: `session/instance.go`, `session/storage.go`, `server/services/event_converter.go`

---

## Phase 4: Frontend

### Epic 4.1: Artifacts Tab
**Goal**: Add an "Artifacts" tab to `SessionDetailView` that renders PR links, commit SHAs, and external URLs from `session.artifacts`.

#### Story 4.1.1: ArtifactsTab component
**As a** developer, **I want** a dedicated Artifacts tab in the session detail view that shows rich links, **so that** I can navigate to PRs and commits created by the session without reading raw terminal output.

**Acceptance Criteria**:
- `ArtifactsTab` renders three sections: "Pull Requests", "Commits", "External URLs"
- Each PR URL is a clickable anchor (`<a href target="_blank">`) formatted as `owner/repo#N`
- Commit SHAs are rendered as 7-char shortened links to `https://github.com/...` if owner/repo known, else monospace text
- External URLs are clickable, truncated to 60 chars in display
- Empty state: "No artifacts found yet — extraction runs automatically in the background."
- Tab is visible only when `session.artifacts` is non-null (or always visible with empty state)

**Files**:
- `web-app/src/components/sessions/ArtifactsTab.tsx` (new file)
- `web-app/src/components/sessions/ArtifactsTab.css.ts` (new file)

##### Task 4.1.1a: ArtifactsTab component (~5 min)
**NOTE from UX review**: Three design changes from the original draft:
1. Three distinct empty sub-states: `artifacts === undefined` → "Extraction pending", all-empty arrays → "No artifacts found", scanning now → footer "Scanning now…"
2. External URLs collapse behind a disclosure toggle by default (they are noisy)
3. CSS token/import path bugs fixed in Task 4.1.1b

- Create `web-app/src/components/sessions/ArtifactsTab.tsx`:
  ```tsx
  // +feature: session-artifacts
  "use client";

  import { useState } from "react";
  import { Session } from "@/gen/session/v1/types_pb";
  import * as styles from "./ArtifactsTab.css";

  interface ArtifactsTabProps {
    session: Session;
  }

  function parsePRDisplay(url: string): string {
    const m = url.match(/github\.com\/([\w.-]+\/[\w.-]+)\/pull\/(\d+)/);
    return m ? `${m[1]}#${m[2]}` : url;
  }

  export function ArtifactsTab({ session }: ArtifactsTabProps) {
    const [urlsExpanded, setUrlsExpanded] = useState(false);
    const artifacts = session.artifacts;

    // Sub-state 1: scan not yet run (server omits the field entirely)
    if (!artifacts) {
      return (
        <div className={styles.emptyState}>
          <span>Extraction pending — will populate automatically once the session starts.</span>
        </div>
      );
    }

    const hasContent =
      artifacts.prUrls.length > 0 ||
      artifacts.commitShas.length > 0 ||
      artifacts.externalUrls.length > 0;

    // Sub-state 2: scan ran but found nothing
    if (!hasContent) {
      return (
        <div className={styles.emptyState}>
          <span>No artifacts found in this session&apos;s conversation history.</span>
        </div>
      );
    }

    return (
      <div className={styles.container}>
        {artifacts.prUrls.length > 0 && (
          <section className={styles.section}>
            <h3 className={styles.sectionTitle}>Pull Requests</h3>
            <ul className={styles.list}>
              {artifacts.prUrls.map((url) => (
                <li key={url}>
                  <a href={url} target="_blank" rel="noopener noreferrer" className={styles.link}>
                    {parsePRDisplay(url)}
                  </a>
                </li>
              ))}
            </ul>
          </section>
        )}
        {artifacts.commitShas.length > 0 && (
          <section className={styles.section}>
            <h3 className={styles.sectionTitle}>Commits</h3>
            <ul className={styles.list}>
              {artifacts.commitShas.map((sha) => (
                <li key={sha}>
                  <code className={styles.sha}>{sha.slice(0, 7)}</code>
                  <span className={styles.shaFull}> {sha}</span>
                </li>
              ))}
            </ul>
          </section>
        )}
        {artifacts.externalUrls.length > 0 && (
          <section className={styles.section}>
            <h3 className={styles.sectionTitle}>External URLs</h3>
            {/* Collapsed by default — external URLs are often noisy */}
            {urlsExpanded ? (
              <>
                <ul className={styles.list}>
                  {artifacts.externalUrls.map((url) => (
                    <li key={url}>
                      <a href={url} target="_blank" rel="noopener noreferrer" className={styles.link}>
                        {url.length > 60 ? url.slice(0, 60) + "…" : url}
                      </a>
                    </li>
                  ))}
                </ul>
                <button className={styles.urlToggleButton} onClick={() => setUrlsExpanded(false)}>
                  Hide URLs
                </button>
              </>
            ) : (
              <button className={styles.urlToggleButton} onClick={() => setUrlsExpanded(true)}>
                Show {artifacts.externalUrls.length} external URL{artifacts.externalUrls.length !== 1 ? "s" : ""}
              </button>
            )}
          </section>
        )}
      </div>
    );
  }
  ```
- Files: `web-app/src/components/sessions/ArtifactsTab.tsx`

##### Task 4.1.1b: ArtifactsTab vanilla-extract styles (~3 min)
**NOTE from UX review**: Two bugs in the original draft are fixed below:
1. Import path is `"../../styles/theme-contract.css"` (NOT `theme.css`) 
2. Link color token is `vars.color.primary` (NOT `vars.color.actionPrimary` — doesn't exist in theme contract)

Verify the correct token names by reading `web-app/src/styles/theme-contract.css.ts` before coding.

- Create `web-app/src/components/sessions/ArtifactsTab.css.ts`:
  ```ts
  import { style } from "@vanilla-extract/css";
  import { vars } from "../../styles/theme-contract.css";

  export const container = style({
    height: "100%",
    overflowY: "auto",
    padding: vars.space[4],
    display: "flex",
    flexDirection: "column",
    gap: vars.space[6],
  });

  export const emptyState = style({
    padding: vars.space[6],
    color: vars.color.textSecondary,
    textAlign: "center",
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
    gap: vars.space[2],
  });

  export const section = style({});

  export const sectionTitle = style({
    fontSize: vars.fontSize.sm,
    fontWeight: 600,
    color: vars.color.textSecondary,
    textTransform: "uppercase",
    letterSpacing: "0.05em",
    marginBottom: vars.space[2],
  });

  export const list = style({
    listStyle: "none",
    padding: 0,
    margin: 0,
    display: "flex",
    flexDirection: "column",
    gap: vars.space[1],
  });

  export const link = style({
    color: vars.color.primary, // NOT vars.color.actionPrimary — verify token exists
    textDecoration: "none",
    fontSize: vars.fontSize.sm,
    selectors: {
      "&:hover": { textDecoration: "underline" },
    },
  });

  export const sha = style({
    fontFamily: "monospace",
    fontSize: vars.fontSize.sm,
    color: vars.color.textPrimary,
    marginRight: vars.space[2],
  });

  export const shaFull = style({
    fontSize: vars.fontSize.xs,
    color: vars.color.textMuted,
    fontFamily: "monospace",
  });

  export const urlToggleButton = style({
    background: "none",
    border: "none",
    cursor: "pointer",
    color: vars.color.primary,
    fontSize: vars.fontSize.sm,
    padding: `${vars.space[1]} 0`,
    selectors: {
      "&:hover": { textDecoration: "underline" },
    },
  });
  ```
- Files: `web-app/src/components/sessions/ArtifactsTab.css.ts`

---

#### Story 4.1.2: Wire Artifacts tab into SessionDetailView
**As a** user, **I want** to click an "Artifacts" tab in the session detail panel, **so that** I can see structured artifacts without leaving the app.

**Acceptance Criteria**:
- `SessionDetailTab` union includes `"artifacts"`
- Tabs array in `SessionDetailView` includes `{ id: "artifacts", label: "Artifacts", icon: Package }` (lucide `Package` icon)
- Clicking the tab renders `<ArtifactsTab session={session} />`
- Tab is always present (not conditionally hidden); empty state handles the no-data case

**Files**:
- `web-app/src/components/sessions/SessionDetail.tsx` (type union update)
- `web-app/src/components/sessions/SessionDetailView.tsx` (tab + render)

##### Task 4.1.2a: Add "artifacts" to tab union and render (~5 min)
- Open `web-app/src/components/sessions/SessionDetail.tsx`
- Change `export type SessionDetailTab = "terminal" | "diff" | "vcs" | "logs" | "info" | "files" | "browser";`
  to `... | "artifacts";`
- Open `web-app/src/components/sessions/SessionDetailView.tsx`
- Add import: `import { Package } from "lucide-react";` and `import { ArtifactsTab } from "./ArtifactsTab";`
- In the `tabs` array (around line 260), append:
  ```ts
  { id: "artifacts" as SessionDetailTab, label: "Artifacts", icon: Package },
  ```
- In the tab content render block (find the `activeTab === "info"` etc. conditionals), add:
  ```tsx
  {activeTab === "artifacts" && <ArtifactsTab session={session} />}
  ```
- Files: `web-app/src/components/sessions/SessionDetail.tsx`, `web-app/src/components/sessions/SessionDetailView.tsx`

---

## Phase 5: Tests

### Epic 5.1: Test Coverage
**Goal**: Achieve tested coverage for the extractor logic, frontend component, and one e2e smoke test.

#### Story 5.1.1: Go extractor unit tests
**As a** developer, **I want** unit tests for the artifact extraction regexes and scan logic, **so that** false-positive and incremental-scan correctness is verified without running the full server.

**Acceptance Criteria**:
- `TestExtractFromToolResult_PRURLs` verifies PR URL extraction from tool_result text
- `TestExtractFromToolResult_NoPRFromAssistantText` verifies assistant text produces no hits (use a sample assistant JSON line)
- `TestExtractFromToolResult_CommitSHAs` verifies 40-hex extraction
- `TestScanFile_IncrementalOffset` verifies second scan on appended file reads only new bytes (check offset advances correctly)
- All tests pass under `go test ./session/artifacts/...`

**Files**:
- `session/artifacts/extractor_test.go` (new file)
- `session/artifacts/store_test.go` (new file)

##### Task 5.1.1a: Write extractor unit tests (~5 min)
- Create `session/artifacts/extractor_test.go`:
  - `TestExtractFromToolResult_PRURLs`: input `"Created PR: https://github.com/owner/repo/pull/42"` → `prURLs == ["https://github.com/owner/repo/pull/42"]`
  - `TestExtractFromToolResult_SkipsAssistantText`: pass text from an assistant message body (not tool_result) → all slices empty (since callers must filter; test the regex directly — it will match, so the real guard is at the call site in `scan.go`)
  - `TestExtractFromToolResult_CommitSHA`: input containing `"abc123def456abc123def456abc123def456abc1"` → commitSHAs has one entry
  - `TestExtractFromToolResult_ExternalURLCap`: generate 60 URLs → `cap50` returns 50
- Files: `session/artifacts/extractor_test.go`

##### Task 5.1.1b: Write incremental scan test (~4 min)
- Create `session/artifacts/store_test.go`:
  - `TestScanFile_IncrementalOffset`: write a temp JSONL with 2 user+tool_result lines, scan, check offsets map; append 2 more lines, scan again — verify offset advanced by exactly the new bytes
  - Use a mock `storeFn` and `lookupTitle` to capture calls
- Files: `session/artifacts/store_test.go`

---

#### Story 5.1.2: Frontend ArtifactsTab unit tests
**As a** developer, **I want** Jest tests for the ArtifactsTab component, **so that** rendering of PR links, commits, and the empty state is verified.

**Acceptance Criteria**:
- `ArtifactsTab_should_showEmptyState_When_artifactsIsNull`
- `ArtifactsTab_should_renderPRLinks_When_artifactsHasPRURLs`
- `ArtifactsTab_should_truncateURLs_When_URLExceeds60Chars`
- All tests pass under `cd web-app && npx jest --no-coverage --testPathPatterns="ArtifactsTab"`

**Files**:
- `web-app/src/components/sessions/__tests__/ArtifactsTab.test.tsx` (new file)

##### Task 5.1.2a: Write ArtifactsTab Jest tests (~5 min)
- Create `web-app/src/components/sessions/__tests__/ArtifactsTab.test.tsx`:
  ```tsx
  import { render, screen } from "@testing-library/react";
  import { ArtifactsTab } from "../ArtifactsTab";
  import { Session } from "@/gen/session/v1/types_pb";

  describe("ArtifactsTab", () => {
    it("ArtifactsTab_should_showEmptyState_When_artifactsIsNull", () => {
      const session = new Session({ artifacts: undefined });
      render(<ArtifactsTab session={session} />);
      // When artifacts is undefined the component renders "Extraction pending"
      expect(screen.getByText(/Extraction pending/)).toBeInTheDocument();
    });

    it("ArtifactsTab_should_renderPRLinks_When_artifactsHasPRURLs", () => {
      const session = new Session({
        artifacts: { prUrls: ["https://github.com/owner/repo/pull/42"], commitShas: [], externalUrls: [] },
      });
      render(<ArtifactsTab session={session} />);
      expect(screen.getByText("owner/repo#42")).toBeInTheDocument();
    });

    it("ArtifactsTab_should_truncateURLs_When_URLExceeds60Chars", () => {
      const longURL = "https://example.com/" + "a".repeat(60);
      const session = new Session({
        artifacts: { prUrls: [], commitShas: [], externalUrls: [longURL] },
      });
      render(<ArtifactsTab session={session} />);
      expect(screen.getByText(/…$/)).toBeInTheDocument();
    });
  });
  ```
- Files: `web-app/src/components/sessions/__tests__/ArtifactsTab.test.tsx`

---

#### Story 5.1.3: e2e smoke test
**As a** QA engineer, **I want** a Playwright test that verifies the Artifacts tab is visible in the session detail view, **so that** the feature is confirmed wired end-to-end.

**Acceptance Criteria**:
- Test navigates to a session detail view
- Clicks the "Artifacts" tab
- Asserts the tab panel is visible (either empty state or artifact content)
- Test ID: `T-E2E-ARTIFACTS-001`

**Files**:
- `tests/e2e/session-artifacts.spec.ts` (new file)

##### Task 5.1.3a: Write Playwright smoke test (~4 min)
- Create `tests/e2e/session-artifacts.spec.ts`:
  ```ts
  // @feature session:artifacts
  import { test, expect } from "@playwright/test";

  test.describe("session-artifacts", () => {
    test("T-E2E-ARTIFACTS-001 Artifacts tab visible in session detail", async ({ page }) => {
      await page.goto("http://localhost:8544");
      // Navigate to first available session
      const firstSession = page.getByRole("listitem").first();
      await firstSession.click();
      // Click Artifacts tab
      await page.getByRole("tab", { name: "Artifacts" }).click();
      // Assert panel is present (empty state or content)
      await expect(
        page.getByText(/No artifacts found yet|Pull Requests|Commits|External URLs/)
      ).toBeVisible();
    });
  });
  ```
- Files: `tests/e2e/session-artifacts.spec.ts`

---

## Registry Updates (required before PR is complete)

Per `.claude/rules/feature-registry.md`, update these files:
- `docs/registry/backend-features.json` — add entry `id: "session:artifacts"`, `tested: true`, `testIds: ["TestExtractFromToolResult_PRURLs", "TestScanFile_IncrementalOffset"]`
- `docs/registry/frontend-features.json` — add entry `id: "session-artifacts"`, `type: "frontend"`, `component: "ArtifactsTab"`, `file: "web-app/src/components/sessions/ArtifactsTab.tsx"`, `tested: true`, `testIds: ["ArtifactsTab_should_showEmptyState_When_artifactsIsNull", "ArtifactsTab_should_renderPRLinks_When_artifactsHasPRURLs"]`

---

## Pitfall Reference

| Pitfall | Mitigation |
|---|---|
| O(n²) re-read | `ScanOffsetBytes` in blob; `f.Seek(offset, io.SeekStart)` |
| False positives from assistant text | Only process `type=="user"` + `content[].type=="tool_result"` |
| Missing `--feature sql/upsert` in ent generate | Documented in CLAUDE.md; use exact command from `session/ent/generate.go` |
| Partial last line panics | `json.Unmarshal` error → rollback byte count, stop scanning |
| `inst.stateMutex` held during I/O | Copy `HistoryFilePath` under brief RLock; release before `os.Open` |
| PR poller race on github_pr_number | Check `inst.GitHubPRNumber == 0` before calling `UpdateInstancePRNumber` |
| ExternalURL list growth | `cap50()` helper; applied in `mergeAndPersist` |
| `position: fixed` overlays | N/A — no new modals in this feature |
| JSONL types unexported | Either export `JSONLEntry`/`JSONLUserMessage` in `session/tokens/jsonl_types.go`, or copy struct definitions locally |
