# Pitfalls: session-history-metadata

Research date: 2026-06-22  
Codebase branch: ssq-pr-integration

---

## 1. JSONL Parsing Pitfalls

### 1.1 Partial (unflushed) lines at end of file

Claude Code appends to the JSONL file mid-conversation. On Linux, `write(2)` is not atomic for large lines; a partial JSON line will appear at the tail of the file at any moment the scanner reads it. The existing `readAllMessagesFromFile` already handles this silently: `json.Unmarshal` errors are discarded (`continue`). The artifact scanner **must** adopt the same pattern — skip unparseable trailing lines rather than treating them as corruption or stopping the scan. A partial line at EOF is the normal operating state, not an error.

**Recommendation:** Mirror `readAllMessagesFromFile`'s `err → continue` approach for every JSON parse error in the artifact scanner. Never return an error when a JSON parse fails on the last line.

### 1.2 O(n²) re-read on every fsnotify WRITE event

`HistoryFileWatcher.handleEvent` fires on `fsnotify.Write`, which is emitted every time Claude appends a message. For a long session this can be many times per minute. If the artifact scanner re-reads the entire file from byte 0 on every WRITE event, the work per event grows linearly with file size, producing O(n²) total work over the session lifetime. A 100 MB file gets fully re-read dozens of times in the last hour of the session.

**Recommendation:** Track a `lastScannedByteOffset int64` per session (stored in memory, not DB). On each wake, `Seek` to that offset, scan only new lines, advance the offset. A file-size cache can gate the work: skip the scan entirely when `stat.Size()` equals the last seen size.

### 1.3 Claude Code JSONL schema assumptions

The codebase already parses `conversationMessage` with fields `type`, `uuid`, `sessionId`, `timestamp`, `cwd`, and `message.{role, model, content}`. The `content` field is typed as `interface{}` precisely because Claude Code encodes it as either a `string` (for text messages) or a `[]ContentBlock` array (for tool-use, tool-result, and mixed messages). URL and PR extraction will need to recurse into both forms. Assumptions that will silently break:

- Treating `content` as always a `string` misses tool-result blocks (which is where `gh pr create` output and `git push` output appear as `tool_result` content).
- The `type` field may be `"summary"` (Claude's internal summary lines) which have a different structure — do not assume `message` is always present.
- Field names are camelCase (`sessionId`, not `session_id`) — a common gotcha when writing new struct tags.
- The schema is undocumented and has changed between Claude Code versions. Guard with a `type` check before accessing nested fields.

**Recommendation:** Define a `type artifactEntry struct` that uses `json.RawMessage` for `content`, then separate the content-extraction step from the line-parsing step. This makes schema evolution cheap (only one place to change).

### 1.4 Concurrent read while Claude Code writes

On Linux, `read(2)` and `write(2)` on the same file from different processes do not require any locking; the kernel provides coherent reads of whatever bytes have been committed. There is no risk of reading a torn message mid-write for any line smaller than the OS write buffer (typically 4 KB). JSONL lines in Claude Code's conversation files are typically 1–50 KB. The only real risk is a partial final line (covered in 1.1). No file locking is needed on Linux. macOS has the same POSIX guarantee. Advisory `flock` would add latency without correctness benefit.

**Recommendation:** No locking needed. Simply tolerate the partial final line.

---

## 2. Ent Schema Migration Pitfalls

### 2.1 Missing `--feature sql/upsert` flag

CLAUDE.md explicitly documents this as critical. The `--feature sql/upsert` flag generates `UpsertRule`, `OnConflict`, and related bulk-upsert helpers on each entity's builder. If the flag is omitted, `UpsertRule` will not exist in the generated code, and any call site that uses `client.SomeEntity.Create().OnConflict(...).DoNothing().Exec(ctx)` will fail to compile. Ent does not warn about this at schema-edit time; the failure is a compile-time `undefined: ent.OnConflict` type error that only appears after regeneration.

**Recommendation:** Never run `go run entgo.io/ent/cmd/ent generate ./session/ent/schema` bare. Alias or wrap it exactly as shown in CLAUDE.md:
```
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```
Add a `Makefile` target (if one does not exist) or document it in a CI lint step.

### 2.2 Nullable TEXT columns vs. a new entity table

**Option A — columns on Session:** Adding `extracted_pr_urls TEXT`, `extracted_commit_shas TEXT`, `extracted_external_urls TEXT` as nullable JSON-encoded blobs is the lightest migration path. Ent's `client.Schema.Create()` (called at startup in `NewEntRepository`) will `ALTER TABLE session ADD COLUMN` automatically for new nullable fields. No data loss risk for existing rows. The downside: these columns are not queryable (you can't `WHERE extracted_pr_urls LIKE '%github%'`) and will balloon the `sessions` table row size.

**Option B — a new `SessionArtifact` entity:** A separate table with `(session_title, artifact_type, url, discovered_at)` is queryable, compact per-row, and extensible. The migration is also automatic via `client.Schema.Create()` because it creates the new table from scratch. The risk: the new entity needs proper `edge` wiring back to `Session`, and the `--feature sql/upsert` flag must be present (see 2.1) if the scanner uses upsert-on-conflict to avoid re-inserting duplicates.

**Recommendation:** Use Option B (new entity). The queryability and extensibility outweigh the added schema complexity. Wire it as `edge.To("artifacts", SessionArtifact.Type)` from `Session`, and use `UpsertOne` with `OnConflict(sql.ConflictColumns("session_id", "url")).DoNothing()` to make the scanner idempotent.

### 2.3 `UpdateGitHubPRNumber` uses `session.Title` as key — fragility

`UpdateGitHubPRNumber` (line 772, `ent_repository.go`) and `UpdateGitHubPRURL` look up sessions by `session.Title`, not by a surrogate primary key. `Title` is marked `Unique` in the schema, which is correct. However, if a session is renamed (currently not a feature but plausible future work), any artifact rows keyed by the old title would orphan. If artifacts are stored in a new entity table, they should use the ent `Session` row ID (an integer auto-increment `id` column that ent creates by default) as the foreign key, not the `title` string.

**Recommendation:** The new `SessionArtifact` entity must use the ent-generated `session_id` integer FK (the `edge.From` back-reference), not `session.Title`. This is already how `ClaudeSession`, `Worktree`, `DiffStats`, and `Tags` are wired.

---

## 3. Concurrency Pitfalls

### 3.1 Scanner goroutine calling back into Storage while HistoryLinker holds its lock

`HistoryLinker.mu` is a `deadlock.RWMutex`. The existing `extraCallbacks` pattern (used for `TokenStore`) fires the callback **after** the write lock is released:
```go
hl.mu.Lock()
// ... mutate state ...
hl.mu.Unlock()

hl.mu.RLock()
cbs := hl.extraCallbacks
hl.mu.RUnlock()
for _, cb := range cbs { cb(filePath) }  // called outside both locks
```
This is the correct pattern. If an artifact-scanner callback were instead registered inside `ScanAll()` and called while `hl.mu` was still held, and the callback then called `storage.UpdateSession(...)` which blocks on the SQLite write connection (serialized via `db.SetMaxOpenConns(1)`), and if `Storage.SomeMethod` also tried to acquire `hl.mu` for any reason, a deadlock would result.

**Recommendation:** Wire the artifact scanner as an `extraCallback` using `RegisterFileCallback`, matching the existing `TokenStore` pattern. Never call back into any method that might re-acquire `hl.mu` from inside a callback invoked while `hl.mu` is held.

### 3.2 Race between PRStatusPoller and artifact scanner writing github_pr_number

`PRStatusPoller.fetchAndUpdatePRStatus` writes `storage.UpdateInstancePRNumber` (via `pr_status_poller.go`). If the artifact scanner also writes `github_pr_number` (from parsing a `gh pr create` URL in the JSONL), both goroutines can be in-flight simultaneously for the same session. With `db.SetMaxOpenConns(1)` and SQLite WAL mode, the writes are serialized at the DB level — no data corruption occurs. But the last-writer-wins semantic means one update can silently overwrite the other.

**Recommendation:** Deduplicate at the application level: before writing, check `inst.GitHubPRNumber > 0`. If it is already set, skip the artifact-scanner write. If not set, write. This mirrors the existing `prURLLinked` guard in `session_driver.go` (line 133). Alternatively, if the scanner stores URLs in a separate `SessionArtifact` table rather than in `github_pr_number`, this conflict disappears entirely.

### 3.3 `inst.stateMutex` lock ordering with scanner I/O

`instance.stateMutex` is an `RWMutex` (deadlock-detected). Multiple callers in `session_driver.go`, `instance_approval.go`, and `instance_checkpoint.go` acquire it. The explicit comment in `git_worktree_manager.go` line 169 states: "This method performs I/O and should be called WITHOUT holding `Instance.stateMutex`." The same rule applies to the artifact scanner: it reads the `HistoryFilePath` field from `inst`, then does file I/O. It must read `inst.HistoryFilePath` under a brief `stateMutex.RLock`, copy the value, release the lock, then do the file read outside the lock. Never hold `stateMutex` while doing disk I/O.

**Recommendation:** Adopt the copy-then-release pattern:
```go
inst.stateMutex.RLock()
path := inst.HistoryFilePath
inst.stateMutex.RUnlock()
if path == "" { return }
// file I/O here, no lock held
```

---

## 4. False Positive Pitfalls

### 4.1 URLs in code snippets vs. URLs the agent actually visited

Claude's assistant messages frequently contain code examples with URLs (`// see https://pkg.go.dev/...`, `curl https://api.example.com`, markdown links in explanations). These are qualitatively different from URLs the agent actually navigated to via a tool call. The content structure distinguishes them:

- **Tool-result blocks** (`type: "tool_result"`, `tool_use_id: "toolu_..."`) contain the actual output of `WebFetch`, `Bash` (with `curl`/`gh`), or browser tools. URLs here are likely "visited."
- **Assistant text messages** (`role: "assistant"`, `content` is a string or text block) contain explanations. URLs here are likely "mentioned."

**Recommendation:** Only extract URLs from `tool_result` content blocks, not from assistant text blocks. Additionally, scope PR URL extraction to messages where the tool name is `Bash` or `computer` and the output contains `github.com/.*/pull/\d+`. This requires inspecting `tool_use` → `tool_result` pairs, not just the raw URL.

### 4.2 Commit SHA collisions with other hex strings

40-character hex strings appear in: git commit SHAs, Docker image digests (`sha256:abc123...` but those are 64-char), file content hashes, session UUIDs (though those are hyphenated). A bare 40-char hex string in a JSONL message is ambiguous. Even if it is a real git SHA, it may reference a repository unrelated to the session's worktree.

**Recommendation:** Only extract commit SHAs when they appear in patterns that contextually imply git:
- Adjacent to `git log`, `git show`, `git push`, `git cherry-pick` in tool input/output.
- In a `tool_result` where the corresponding `tool_use` had a `command` field containing a git subcommand.
- Prefixed by known patterns: `commit `, `HEAD is now at `, `Merge commit '`.
Do not extract standalone 40-char hex strings.

### 4.3 `#123` as issue reference vs. PR reference

GitHub uses `#N` for both issues and pull requests. In commit messages (`git log` output), `#123` is almost always an issue or PR reference but the distinction requires a GitHub API call to resolve. In `gh pr create` / `gh pr view` output, the number is always a PR.

**Recommendation:** Do not attempt to infer PR numbers from bare `#N` patterns in commit messages. Only extract PR numbers from full PR URLs (`https://github.com/owner/repo/pull/N`) or from `gh pr` tool output that explicitly contains `pull_request` JSON. Use the URL's canonical path to derive the number (`/pull/N` → `N`), not regex on `#N` strings.

---

## 5. Frontend Pitfalls

### 5.1 Flash of empty state from a second RPC call

If artifact data is fetched via a separate `GetSessionArtifacts` RPC call (triggered after the session detail page loads), there will be a visible flash where the PR/URL section renders empty or with a skeleton loader before data arrives. For sessions with no artifacts, this flash is indistinguishable from "data is loading" vs. "there are no artifacts."

**Recommendation:** Include artifact data in the main `GetSession` response (embed as a repeated field on the session proto). This makes the page render complete on first paint. If the data grows too large, an alternative is to include only a `has_artifacts: bool` flag in `GetSession` and lazy-load details; but for typical sessions (a handful of PR URLs and commits), embedding is simpler and eliminates the flash.

### 5.2 Real-time updates when the scanner finds a new artifact mid-session

The frontend currently uses ConnectRPC streaming or polling to receive session updates. If the artifact scanner discovers a new PR URL while the user is viewing the session detail page, the frontend has no mechanism to know it should re-fetch artifacts. The EventBus (`onUpdated` callback in `PRStatusPoller`) is the existing pattern for notifying the frontend of PR status changes.

**Recommendation:** Emit a session-updated event via the existing EventBus when the artifact scanner commits a new artifact to the DB. This piggybacks on whatever push mechanism the frontend already uses for session state updates. No new streaming channel is needed.

### 5.3 Unbounded URL list in the UI

Long sessions can accumulate hundreds of external URLs (every web search result, every documentation page visited). Displaying them all in a flat list creates an unusable UI and may cause layout performance issues in the React virtual list.

**Recommendation:** Cap the displayed list at 10–20 URLs with a "show all" expansion, or deduplicate by domain and show domain clusters. Prioritize PR URLs and commit SHAs at the top (they have the highest signal-to-noise ratio) and group external URLs separately. Store a `relevance_score` or `artifact_type` enum in the DB to enable this sorting without re-parsing on the frontend.

---

## 6. Operational Pitfalls

### 6.1 Memory pressure from reading large JSONL files

A long session's JSONL file can reach 50–200 MB. `readAllMessagesFromFile` loads the file line-by-line via `bufio.Scanner` with a 1 MB line buffer — it does not hold the entire file in memory at once. This is safe for sequential full scans. However, if the artifact scanner holds extracted URL strings in memory (e.g., a `[]string` accumulator), and there are thousands of unique URLs across a large session, that accumulator can grow large.

**Recommendation:** Stream-parse and write to DB incrementally rather than accumulating in memory. Use the byte-offset approach (pitfall 1.2) so only new lines since the last scan are read. Bound the in-memory accumulator to ~500 items; flush to DB and clear the slice when it exceeds that threshold.

### 6.2 Re-extracting artifacts already in DB after app restart

At restart, `NewEntRepository` calls `client.Schema.Create()` and then the app starts the HistoryLinker, which calls `ScanAll()`. If the artifact scanner is wired as a `RegisterFileCallback`, it will be called for every JSONL file that the HistoryLinker detects — including files from sessions whose artifacts are already fully stored in the DB from a previous run. A naive scanner will re-read and re-insert duplicates (or waste time checking upserts for thousands of rows).

**Recommendation:** Persist the `lastScannedByteOffset` per session in the DB (a field on `SessionArtifact` or the Session entity itself). At startup, load this offset; skip scanning if `stat.Size() == lastScannedByteOffset`. This makes the restart cost proportional to new bytes written since the last shutdown, not total file size. An alternative is to store a `last_artifact_scan_at` timestamp and only re-scan files modified after that timestamp.

---

## Top 3 Pitfalls to Design Against

1. **O(n²) full re-read on every WRITE event** (pitfall 1.2): The HistoryFileWatcher fires on every append. Without byte-offset tracking, a long session re-reads its entire (possibly 100 MB) JSONL on every Claude message. This is the highest-impact performance bug and the easiest to introduce accidentally. Must be designed out from the start with a persisted `lastScannedByteOffset`.

2. **False URL extraction from assistant text vs. tool results** (pitfall 4.1): Without distinguishing `tool_result` blocks from `assistant` text, the scanner will extract hundreds of mentioned URLs (documentation links, code examples, API references in explanations) as if the agent visited them. This produces noisy, low-quality artifacts and degrades the UI. The `tool_result` / `tool_use` pairing in the JSONL content schema is the correct signal; designing the parser around it from day one avoids a later retroactive re-classification pass.

3. **Missing `--feature sql/upsert` in ent generate** (pitfall 2.1): Adding a new `SessionArtifact` entity and using `OnConflict(...).DoNothing()` for idempotent scanner writes requires the upsert feature. If the generate command is run without the flag (e.g., by a contributor following generic ent documentation), the generated code silently lacks the upsert helpers, causing compile errors that are confusing to diagnose. The Makefile must enforce the flag.
