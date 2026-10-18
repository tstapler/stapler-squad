# Pre-Mortem: RUM Session Diagnostic Bundle
**Date**: 2026-10-16

## Scenario: It is 6 months after launch and this feature has failed. What went wrong?

---

## Failure Modes

### P1 — Critical (launch-blocking if not mitigated)

| # | Failure | Root Cause | Prevention in plan |
|---|---------|------------|-------------------|
| 1 | Server is exploited: an attacker writes a ZIP to `/etc/cron.d/` by passing `output_dir = "../../../../etc/cron.d"` in `SaveSessionBundleRequest` | `req.Msg.OutputDir` is accepted verbatim. Task 1.3.3a resolves the path with `(or req.Msg.OutputDir when non-empty)` and no sanitization or `filepath.Clean`/`strings.HasPrefix(configDir)` guard is specified. | **✅ PATCHED** — Task 1.3.3a updated: `filepath.Clean(filepath.Abs(req.Msg.OutputDir))` + prefix check vs `config.GetConfigDir()/diagnostics/`; return `CodeInvalidArgument` on failure. |
| 2 | Server OOMs under concurrent export load: 10 users simultaneously trigger export of 45 MB sessions, spiking 450 MB of in-memory ZIP bytes on top of normal heap usage | `assembleBundle` returns `[]byte` ZIP assembled entirely in memory; `ExportSessionBundle` calls it before streaming. No semaphore or concurrency cap is described anywhere in the plan. | **✅ PATCHED** — `SessionBundleService` struct now has `exportSem chan struct{}` (capacity 3); `ExportSessionBundle` acquires/releases the slot; returns `CodeResourceExhausted` when all slots are busy. |
| 3 | Diagnostics directory fills disk; server's disk-full guard never fires because retention only runs at startup, and this is a long-lived systemd service | `RunDiagnosticsRetention` is called "during `BuildRuntimeDeps`" (startup). The server runs continuously for weeks between deployments. No periodic ticker is wired. Bundles accumulate until disk exhaustion causes unrelated failures. | **✅ PATCHED** — Task 1.3.3b updated: in addition to the startup call, a goroutine with `time.NewTicker(24h)` is wired in `BuildRuntimeDeps`, matching `analytics/retention.go`'s pattern. |

---

### P2 — High (significant quality/UX risk)

| # | Failure | Root Cause | Prevention in plan |
|---|---------|------------|-------------------|
| 4 | "Download .zip" silently fails on iOS Safari; users blame the feature as broken | `<a download>` is a no-op on iOS — the browser opens the ZIP in-place instead of saving it. Story 2.1.1 implements only the `<a download>` path. The iOS Web Share API alternative is noted only in Unresolved Questions; no story, acceptance criterion, or task covers it. | **Absent as a story** — `useExportBundle.download()` needs a `navigator.canShare` feature-detect: use `navigator.share({ files: [new File([blob], filename)] })` on iOS, fall back to `<a download>` elsewhere. |
| 5 | Web Vitals (LCP) never carry `session.uuid` even when a session is in focus, making session-scoped performance analysis from bundles worthless | LCP fires during initial page paint — well before `SessionDetailView` mounts and calls `setActiveSessionUUID`. The plan's Story 3.1.1 calls this out for document-load spans in general but does not identify LCP specifically as affected. `WebVitalsReporter` reads `getActiveSessionUUID()` at fire-time; for LCP that is always null. | **Absent** — consider capturing the active UUID in `WebVitalsReporter` at `onLCP` callback registration time and re-emitting with the correct UUID after `setActiveSessionUUID` is called, or treat LCP as unattributed and document the limitation. |
| 6 | Backlog item notes reference a bundle file that retention deleted; the investigation record is permanently broken | `RunDiagnosticsRetention` deletes all `*.zip` files older than 30 days by mtime with no awareness of whether any backlog item links to them. A bundle saved on day 1 and linked to a backlog item is deleted on day 31 while the item is still open. | **Absent** — retention should skip files whose SHA-256 hash appears in a backlog item's notes, or extend retention to 90 days to match `contexthistory` retention, or record linked-bundle hashes in a side-file exempt from deletion. |
| 7 | Multiple simultaneous `SaveSessionBundle` calls for the same session UUID write different `.zip` files and both return distinct content hashes, creating duplicate artifacts with no deduplication | Filename includes a timestamp (`<uuid>-<RFC3339>.zip`); two calls in the same second produce identical names and the second write overwrites the first silently (or races on the rename). Multiple calls differing by milliseconds create two files. No idempotency key. | **Absent** — add a per-session-UUID mutex in `SessionBundleService` to serialize concurrent saves, or reject a second save if a bundle for that session exists within the last N minutes. |

---

### P3 — Medium (worth logging, not blocking)

| # | Failure | Root Cause | Prevention in plan |
|---|---------|------------|-------------------|
| 8 | Client receives a structurally valid but corrupt ZIP because a `BundleChunk` was silently dropped or re-ordered in transit | Server-streaming ConnectRPC over HTTP/2 guarantees ordering, but the client reassembly loop in `useExportBundle` has no end-to-end checksum. The `final = true` flag signals stream end but not integrity. A proxy that truncates the last chunk would not be detected. | **Absent** — `BundleChunk` or `manifest.json` should carry the full ZIP's SHA-256; the client should verify before triggering the download. |
| 9 | Bundle's `scrollback` section contains the most-recent 500 PTY lines but the best-effort secrets scan misses a credential formatted as JSON (`"token": "ghp_..."`) because the entropy heuristic only checks bare high-entropy strings | The plan's secrets scan relies on entropy heuristic + AWS key prefix; JSON-wrapped tokens, `.env`-file-style `KEY=value` embedded in command output, and base64-encoded secrets are outside the described patterns. The scan is warn-only and does not block export, so all exposures reach the bundle silently. | **Partially present** — consent notice + warn banner exist, but the scan coverage gap is unacknowledged; document the limitation explicitly in `manifest.json`'s `pii_scan_note` field. |
| 10 | `GetItemSessionBySessionUUID` returns only the most-recent backlog link; a session that transitioned through three backlog items (diagnose → rework → review) silently drops the earlier two from the bundle | Plan's glossary explicitly notes "returns the most-recent item session link only". The bundle manifest records only that single link, misrepresenting the session's full history to the investigator. | **Present (as a documented design choice)** — but the manifest should state `backlog_linkage_truncated: true` when there are more links than the API returns, so consumers know. |
| 11 | Two server replicas started simultaneously during a rolling restart both run `RunDiagnosticsRetention` and race to `os.Remove` the same files, producing spurious "file not found" log errors | Retention uses `os.Remove` with no existence check inside a loop over directory entries; a second concurrent cleaner removes the same file between the lister's scan and the Remove call. Single-instance in practice, but graceful restarts overlap briefly. | **Absent** — `os.Remove` should swallow `os.IsNotExist` errors, which is a one-line fix with no design impact. |
| 12 | `SaveSessionBundle` disk-space check via `syscall.Statfs` fails to compile on Windows hosts (non-WSL2) if anyone attempts a native Windows port | `syscall.Statfs` is not available on `GOOS=windows`. WSL2 is Linux and compiles fine, but the call site has no build-tag guard or platform-agnostic wrapper. | **Absent (low priority)** — wrap in `diskutil.go` with a `//go:build !windows` file and a no-op (always-sufficient) stub for the windows build tag. |

---

## Highest-Risk Item Summary

**Top P1**: `output_dir` path traversal in `SaveSessionBundleRequest` — an unauthenticated (or authenticated but malicious) client can write a ZIP to any server path, including cron directories and SSH authorized_keys; the plan specifies no sanitization at the write site.

**Top P2**: `<a download>` silent no-op on iOS Safari — the download delivery mode is the primary user story and it simply does not work on mobile iOS without the Web Share API fallback that is left only in Unresolved Questions with no implementing story.
