# Pre-Mortem: session-history-metadata

**Date**: 2026-06-22
**Feature**: Async JSONL artifact scanner — PR URLs, commit SHAs, external URLs, command signals stored in ent DB and surfaced in an "Artifacts" tab.
**Scenario**: It is 3 months after launch. The feature has failed. Write up what went wrong.

---

## Failure Mode 1: Stale Instance Snapshot — Post-Startup Sessions Never Get Artifacts

### What happened

The `lookupTitle` closure in `dependencies.go` captured the `instances []*session.Instance` slice at construction time (a slice-header copy). Sessions created after server startup — the majority of sessions for long-running deployments — were never present in that snapshot. Every `lookupTitle` call on a post-startup session returned `("", false)`, causing `scanFile` to silently drop all extracted artifacts. The `OnScanComplete` closure iterated the same stale snapshot and never found the new sessions either.

Users reported that the Artifacts tab was always empty on new sessions. Restarting the server temporarily fixed it (restoring the snapshot to include all current sessions), which made the bug non-reproducible in development where servers are restarted frequently.

### Root cause

Adversarial review (Pass 2, Concern 1) flagged this: `lookupTitle` and `OnScanComplete` close over a captured `instances` slice rather than a live-snapshot closure such as `func() []*session.Instance { return sessionManager.Instances() }`. The fix was listed as a recommendation but not promoted to a blocker, so it was deferred. The implementer misread "Concern (not blocker)" as "safe to defer until post-launch."

### Leading indicator

A test fixture that creates a session *after* calling `Start()` and then writes a synthetic JSONL file, asserting that `storeFn` was called with the new session's title. This test would fail immediately because `lookupTitle` never finds the session. The current test plan (`TestScanFile_IncrementalOffset`) only tests with sessions seeded before `Start()`.

### How to add to the test plan

In `session/artifacts/store_test.go`, add `TestScanFile_PostStartupSession`:

1. Construct `ArtifactExtractor` with a `lookupTitle` that wraps a live slice (using `sync.RWMutex`-protected list or a live closure).
2. Call `Start()`.
3. Append a new session to the live list *after* `Start()` returns.
4. Write a JSONL file referencing that session's history path.
5. Call `OnHistoryFileChanged(filePath)` and drain the queue.
6. Assert `storeFn` was called exactly once with the correct title.

This test should be **blocking** on the implementation checklist.

---

## Failure Mode 2: Byte Offset Corruption on Partial Write — Scanner Silently Eats All Future Input

### What happened

Claude Code appends JSONL lines incrementally as the conversation progresses. The fsnotify `Write` event fires after every append — often mid-line, before the newline terminator is flushed. When `scanFile` ran on a partial write:

1. `bufio.Scanner.Scan()` stopped at the partial line and returned false.
2. `scanner.Err()` returned nil (partial line with valid UTF-8 is not a scanner error).
3. `f.Seek(0, io.SeekCurrent)` returned the full file position *including* the partially consumed bytes the scanner had read-ahead into its internal 10 MB buffer.
4. That position was stored as the new offset.

On the next scan, `f.Seek(offset, io.SeekStart)` skipped past the partial line — which had since become a complete line. The completed line was never processed. If that line contained the `gh pr create` tool_result with the PR URL, the PR URL was permanently lost.

In practice this affected approximately 15% of PR URL extractions on active sessions, silently. Users noticed when PRs opened during a session didn't show up in the Artifacts tab and had to manually copy the URL from the terminal.

### Root cause

The plan correctly identified partial-last-line handling as a risk and stated "partial last line — stop here; do NOT advance offset past it." But the implementation relied on `json.Unmarshal` returning an error on a partial line as the signal to stop. A partial line that happens to be valid JSON (truncated mid-string-value before the closing `"`) does not return an unmarshal error and may produce a malformed artifact. More critically, the offset was set using `f.Seek(0, io.SeekCurrent)` *after the scanner had read ahead* — the scanner's internal buffer pulled bytes beyond the last cleanly parsed line.

### Leading indicator

A test with a JSONL file that is written in two phases: first a line that is complete and valid, then the first 40 bytes of a second line (no newline). After the first `OnHistoryFileChanged` call, assert the offset equals the length of only the first complete line, not the partial second line. Then write the rest of the second line, trigger a second scan, and assert both lines' artifacts appear. If the offset tracking is correct, the second scan processes the second line. If not, the second line is skipped.

### How to add to the test plan

In `session/artifacts/store_test.go`, add `TestScanFile_PartialWrite`:

1. Write one complete JSONL user+tool_result line to a temp file.
2. Append the first 50 bytes of a second JSONL line (no `\n`).
3. Call `scanFile`. Assert that `offsets[filePath]` equals `len(first_line) + 1` (the newline), NOT `len(first_line) + 1 + 50`.
4. Append the rest of the second line and the closing `\n`.
5. Call `scanFile` again. Assert the second line's artifacts are captured.

This test exercises the exact invariant: offset must track only fully-consumed lines, not scanner read-ahead position.

---

## Failure Mode 3: `lookupTitle` O(n) Scan Causes Event-Bus Backpressure Under Load

### What happened

Users with 50+ sessions noticed a new latency pattern: after any JSONL write event, the WatchSessions stream stalled for 200–400 ms before delivering the next terminal update. The stall correlated exactly with the `OnScanComplete` callback firing.

Tracing revealed: `OnScanComplete` iterated the full `instances` slice O(n) on every fsnotify callback. With 50 sessions, each emitting multiple fsnotify `Write` events per second (Claude Code flushes after every token), `OnScanComplete` was called ~20 times/second, each doing a 50-element linear scan followed by `eventBus.Publish`. The event bus became saturated, delaying unrelated session updates.

Additionally, `lookupTitle` did the same O(n) scan twice — once to resolve the path to a title, once inside `OnScanComplete` to find the instance to update. With fsnotify coalescing off (Linux inotify fires for every write syscall), a large active session generated 5–10 `Write` events per second.

### Root cause

The plan acknowledged "O(n) scan on every callback" as partially resolved. The adversarial review noted it was resolved from a lock-race perspective but the O(n) complexity was left standing. No cap was placed on callback frequency; fsnotify `Write` events are not rate-limited. The `ArtifactExtractor.inflight` dedup correctly prevents duplicate *parse* work but does not prevent the O(n) `lookupTitle` scan from running on every enqueue call.

### Leading indicator

A benchmark test `BenchmarkOnHistoryFileChanged_50Sessions` that constructs an extractor with 50 instances and calls `OnHistoryFileChanged` in a loop, measuring ns/op. At 50 sessions with O(n) lookup, the benchmark shows >10 µs/call, and at 200 events/second (10 active sessions × 20 writes/session/second) that saturates well under the event bus capacity.

### How to add to the test plan

1. Add a `BenchmarkOnHistoryFileChanged_50Sessions` in `store_test.go` that asserts p99 latency < 50 µs per callback (sufficient headroom for 200 events/second without saturation).
2. Replace the O(n) scan in `lookupTitle` with an O(1) `map[string]string` (filePath → title) that is updated whenever sessions are added or removed. Wire the map update into `HistoryLinker.AddInstance`/`RemoveInstance` callbacks.
3. Include the map-vs-scan latency comparison in the test comment so future reviewers understand why the map was introduced.

---

## Failure Mode 4: `SeedOffsets` Name/Signature Mismatch — Compilation Failure at Integration

### What happened

The server failed to build at the wiring step. `Task 3.1.1b` called `artifactExtractor.SeedOffsets(instances)` where `instances` is `[]*session.Instance`. `Task 2.1.3b` defined the method as `seedOffsetsFromDB(instances []instanceSnapshot)` — unexported, and accepting a different slice type. The build produced:

```
./dependencies.go:791:25: artifactExtractor.SeedOffsets undefined (type *artifacts.ArtifactExtractor has no field or method SeedOffsets)
```

The adversarial review (Pass 2, Concern 2) documented this as a compilation failure. It was marked "not a blocker" in a second-pass context where blockers were the 4 original structural issues. The implementer treating "Concern" as lower priority than "Blocker" deferred it. At wiring time it became a build-stopping error that required understanding the plan well enough to reconcile the two diverging signatures — something that took a junior contributor most of a day.

### Root cause

The plan was written across multiple revision passes, and the method was renamed and its signature changed between the extractor design (Task 2.1.3b) and the wiring step (Task 3.1.1b) without a corresponding update to the earlier task. The adversarial review caught it but the severity label ("Concern") undersold the actual impact (build failure).

### Leading indicator

A `go build ./...` step as a CI gate immediately after the extractor package is first committed, before the wiring step. Any compilation error surfaces within seconds. Alternatively, a stub test file in `session/artifacts/store_test.go` that imports the package and calls `SeedOffsets` with the expected signature — this forces the type error into the test suite on the first `go test` run.

### How to add to the test plan

Add to the Phase 2 checklist (before Phase 3 begins):
- `go build ./session/artifacts/...` passes — verifies the extractor package compiles in isolation.
- Add a compile-time interface assertion in `session/artifacts/store.go`:
  ```go
  var _ interface {
      SeedOffsets(instances []*session.Instance)
      Start(ctx context.Context, historyDir string)
      Stop()
      OnHistoryFileChanged(filePath string)
  } = (*ArtifactExtractor)(nil)
  ```
  This fails at compile time if any exported method has a wrong name or signature.

---

## Failure Mode 5: Regex False Positives — SHA Extraction Pollutes Commit List with Non-Commit Hashes

### What happened

The Artifacts tab Commits section filled up with hundreds of entries, most of them not Git commit SHAs. Users lost trust in the feature entirely.

The `reCommitSHA = regexp.MustCompile(`\b[0-9a-f]{40}\b`)` regex is correct for Git SHAs but also matches:
- Base64-encoded content slices that happen to be 40 hex chars (common in binary file diffs)
- Package lock file integrity hashes (`sha1` in `package-lock.json`, `yarn.lock` tool outputs)
- OpenSSL certificate fingerprints in `curl` TLS negotiation outputs
- Docker image digests (SHA-256 but first 40 chars are hex)

When an agent ran `npm install` in a project with a complex `package-lock.json`, the tool_result contained thousands of integrity hashes. Each matched the SHA regex. After dedup, the Artifacts tab showed 50 unique-but-wrong "commit SHAs."

### Root cause

The regex was designed assuming tool_result output is predominantly Git CLI output. No context heuristic was applied — the plan did not specify that commit SHAs must follow a keyword like `[main `, `commit `, `HEAD at `, or appear in `git commit` / `git push` tool_result lines specifically. The false-positive rate was not tested against realistic tool_result content (package manager output, curl output, build logs).

### Leading indicator

A test `TestExtractFromToolResult_NoFalsePositivesFromNPMOutput` that feeds a realistic `npm install --verbose` tool_result (containing lock file integrity hashes and package checksums) into `ExtractFromToolResult` and asserts `commitSHAs` is empty. This test would fail immediately with the unqualified SHA regex.

### How to add to the test plan

1. Add the following test cases to `extractor_test.go`:
   - `TestExtractFromToolResult_NoFalsePositivesFromNPMOutput`: npm install output → 0 commitSHAs
   - `TestExtractFromToolResult_NoFalsePositivesFromCurlTLS`: curl verbose output → 0 commitSHAs
   - `TestExtractFromToolResult_CommitSHA_ValidGitOutput`: `git push` output containing `abc123...def456 (main -> main)` → 1 commitSHA

2. Change the SHA extraction strategy: instead of `\b[0-9a-f]{40}\b` globally, require context. Options (in ascending complexity):
   - Match only lines where the preceding text contains `commit`, `HEAD`, or `→` (git push summary format)
   - Process only tool_results where the corresponding `tool_use.name == "Bash"` and `tool_use.input.command` contains `git commit` or `git push`
   - Use the existing `CommandArtifact` from `ExtractFromBashCommand` as the primary commit signal, and only confirm SHAs from matching `tool_result` lines

3. Document the chosen strategy in `extractor.go` with a comment explaining why the naive regex was rejected.

---

## Failure Mode 6: JSON Blob Unbounded Growth — `external_urls` Cap Doesn't Protect Against `pr_urls` and `commit_shas`

### What happened

A long-running session (72 hours, 3000+ JSONL lines) that repeatedly ran `git fetch`, `git log --oneline`, and browsed many GitHub pages accumulated 2,400 unique external URLs (the cap was 50 — they were deduplicated but a new batch of 50 different URLs arrived on every merge scan), 380 unique commit SHAs (every `git log` output had new 7-char prefixes that expanded to 40-char via context), and 12 unique PR URLs.

The `session_artifacts` TEXT column grew to 1.8 MB for this single session. On the session list page, which loaded artifact blobs for all sessions in a single DB query to populate the WatchSessions stream, the total payload for 30 sessions averaged 4 MB. The ConnectRPC stream stalled loading the session list.

There was no cap on `pr_urls` or `commit_shas`. The `maxExternalURLs = 50` cap applied only to external URLs.

### Root cause

The design assumed PR URLs and commit SHAs would be small in count (typically < 10 per session). This holds for most sessions but not for long-lived sessions that repeatedly run `git log`, browse many PRs as reference material, or work across large multi-repo monorepos. No cap was enforced on these two artifact types, and the blob was never trimmed during merge.

### Leading indicator

A test `TestMergeAndPersist_SHACap` that simulates 100 scan iterations each adding 20 new commit SHAs, asserting the blob's `CommitSHAs` length never exceeds a defined cap (e.g., 200). Without the cap, the blob grows unboundedly and the test documents the growth rate explicitly.

### How to add to the test plan

1. Add `maxPRURLs = 30` and `maxCommitSHAs = 200` constants to `types.go` alongside `maxExternalURLs`.
2. Apply caps in `mergeAndPersist` using the same `cap50`-style helper.
3. Add `TestMergeAndPersist_BlobSizeIsBounded` that verifies all three lists stay within their caps after 1000 simulated scan iterations.
4. Add a `TestMergeAndPersist_BlobJSONSizeIsBounded` that marshals the capped blob and asserts `len(jsonBytes) < 50_000` (50 KB per session is reasonable for the session list query).

---

## Failure Mode 7: e2e Test Flakes on Empty Session List — CI Is Green but Feature Is Untested

### What happened

The Playwright smoke test (`T-E2E-ARTIFACTS-001`) consistently passed in CI but never actually tested the Artifacts tab functionality. The test relied on:
```ts
const firstSession = page.getByRole("listitem").first();
await firstSession.click();
```

The e2e test server (`STAPLER_SQUAD_INSTANCE=e2e-local`) started with no sessions. `getByRole("listitem").first()` selected the empty-state placeholder element — a `<li>` rendered by the session list when there are no sessions — not a real session. The subsequent `getByRole("tab", { name: "Artifacts" })` then failed to find a tab and the test's `expect` matched the empty-state text that happened to contain "No" as a substring of the no-sessions message.

The test passed vacuously. The feature shipped with a working empty-state renderer but the actual extraction pipeline was broken (due to Failure Mode 1 — post-startup sessions). Neither failure surfaced in CI because the smoke test never exercised a real session with real JSONL output.

### Root cause

The adversarial review (Pass 2, Minor 3) noted: "e2e test clicks first session with no guard for empty list — `page.getByRole("listitem").first()` will fail on a fresh test instance. Add a fixture or skip guard." This was downgraded to a minor and not fixed before launch. The test was green in CI, which gave false confidence. No one verified that "green" meant the feature was actually exercised.

### Leading indicator

A CI step that runs the e2e test against a fresh instance *and* logs which elements the test interacted with (e.g., via `page.on('console', ...)` or Playwright trace). The trace would show the test clicked the empty-state `<li>` element, not a session entry. Alternatively, a fixture assertion: `await expect(page.getByRole("listitem")).toHaveCount({ min: 1 })` before clicking.

### How to add to the test plan

1. Add a test fixture to `tests/e2e/session-artifacts.spec.ts` that creates a session via the API before the test runs, using the existing ConnectRPC `CreateSession` endpoint, and tears it down in `afterEach`.
2. Replace `getByRole("listitem").first()` with `getByTestId("session-list-item").first()` (requires `data-testid` on session list items — already enforced by the e2e conventions in CLAUDE.md).
3. Add an explicit assertion: `await expect(page.getByRole("tab", { name: "Artifacts" })).toBeVisible()` before clicking.
4. Write a JSONL fixture file containing a known PR URL (`https://github.com/test-owner/test-repo/pull/1`) to the test session's expected history path, so the scan runs and the tab shows real content rather than only the empty state.

---

## Summary

The top 3 failure modes by probability of actually occurring, based on what the adversarial review flagged and what matches known patterns in this codebase:

- **Failure Mode 1 (Stale Instance Snapshot)**: Highest probability. The adversarial review documented this as a correctness hole — sessions created post-startup are invisible. Long-running deployments (the primary use case) will experience this 100% of the time after the first session is created. The fix is a one-line change (live closure vs. captured slice) but was not enforced as a blocker.

- **Failure Mode 4 (SeedOffsets Compilation Failure)**: High probability of causing a day-or-more delay at integration time. The adversarial review documented the name/signature mismatch explicitly. Without a compile-time interface assertion or an early `go build` gate, this fails silently until Phase 3 wiring, at which point the implementer must understand both diverging signatures to reconcile them.

- **Failure Mode 5 (SHA Regex False Positives)**: High probability of user-visible quality failure. The unqualified 40-hex regex will match non-Git content in any session that runs `npm install`, `curl`, or `docker pull`. This is the most likely cause of users dismissing the Artifacts tab as "noise" and ignoring it entirely, making the feature functionally dead even when it works technically.
