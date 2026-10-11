# S6: go-git reads vs concurrent repack/gc/pack-refs (Gate G6)

Environment: go1.26.6 darwin/arm64, git 2.50.1 (Apple Git-155), go-git v5.19.2. Source and raw output: `S6-src/` (`main.go`, `runall.sh`, `results.txt`). Scratch repo `/tmp/s6repo`: 50 commits, `git repack -ad` first.

Method: 1 goroutine loops the maintenance command; 8 goroutines loop `repo.CommitObject(HEAD)` + `.Tree()`. Modes: `stock` (one shared `*git.Repository`), `fresh` (`git.PlainOpen` per read, no retry), `fresh-retry` (fresh open per attempt, up to 5 attempts), `reindex-retry` (shared handle, `Storage.Reindex()` then retry, unsynchronised), `reindex-retry-locked` (same, behind a mutex).
Commands: `go build -o s6 . && ./s6 -mode <mode> -maint <repack|gc|packrefs> -dur <d>`.

## Results (VERIFIED, from `S6-src/results.txt` and the console runs)

| mode | maint | dur | reads | ErrObjectNotFound | retried reads | failures after retry |
|---|---|---|---|---|---|---|
| stock (5 s smoke) | repack | 5 s | 52,464 | 333 | - | - |
| stock | repack | 60 s | 97,706 | 0 | - | - |
| stock (3 reruns) | repack | 8 s | 77,227 / 26,373 / 28,717 | 37 / 0 / 934 | - | - |
| fresh, no retry | repack | 60 s | 9,335 | 15 | - | - |
| **fresh-retry** | repack | 60 s | 6,642 | **0** | 16 (max 3 attempts; p50 208 ms, p99 1.05 s, max 1.34 s) | **0** |
| reindex-retry (unlocked) | repack | 60 s | - | **panic: assignment to entry in nil map** (`object.go:97 loadIdxFile`, via `requireIndex`) | | |
| reindex-retry-locked | repack | 60 s | 115,861 | 0 | 0 | 0 |
| stock | gc --prune=now --aggressive | 20 s | 150,628 | 265 | - | - |
| **fresh-retry** | gc | 20 s | 8,601 | **0** | 16 (max 3 attempts; p50 10 ms, p99 115 ms, max 153 ms) | **0** |
| stock | pack-refs --all --prune (+ ref churn) | 20 s | 110,711 | 16 | - | - |
| fresh-retry | pack-refs | 20 s | 13,496 | 0 | 0 (retry never needed) | 0 |

## Findings

1. **Reproduced.** Stock go-git returns `plumbing.ErrObjectNotFound` ("object not found") during concurrent `repack -ad`, `gc` and `pack-refs`. It is non-deterministic: the 60 s stock run saw 0 while 8 s runs saw 37 and 934. Once a shared handle is stale the errors repeat (934 in one 8 s run), consistent with the cached pack index (`object.go:53-71`). `ErrObjectNotFound` was the only error class; no other fs errors surfaced.
2. **Wrapper using only the public API works.** A fresh `PlainOpen` per attempt plus retry gave 0 residual errors in repack and gc runs. Unwrapped fresh opens still failed 15 times, so the retry is necessary, not just the fresh handle.
3. **Plan correction: retry once is not enough.** Max attempts observed was 3. Use a bounded retry of at least 3 attempts (suggest 5, short backoff). Retry latency is high under repack: p99 about 1 s, max 1.34 s (in an environment where 8 readers plus repack saturate the machine; fresh opens also cut read throughput ~15x vs the shared handle: 6.6k vs 97k reads/60 s).
4. **`Reindex()` on a shared handle is unsafe unlocked.** It crashed with a nil-map panic (`loadIdxFile`), confirming the plan note. The mutex-serialised variant had zero failures, but it also recorded zero retries, so this run does not show that Reindex fixes a stale handle; do not rely on it. Not recommended anyway (serialises all reads per repo).
5. `pack-refs` does not move objects; stock still hit 16 not-found (likely a ref/object timing effect while refs are rewritten), and the fresh-retry run needed no retries. Not enough evidence to characterise further.

## Gate G6 verdict

**Wrapper works: F2 stays in-repo (no fork storage patch).** Rule for the `gogit` backend: fresh `*git.Repository` per call via `OpenRepo`, retry on `plumbing.ErrObjectNotFound` up to at least 3 attempts (recommend 5, with ~10-50 ms backoff), never cache a handle. Reads that still fail after the bound fall back to CLI by reason `object_not_found`. Story 4.2.2 (F2) is not needed.

Caveats (INFERRED / not tested): repo size was 50 commits; larger repos make fresh opens and retries slower. Only `CommitObject`+`Tree` reads were exercised, not blob/diff/walk paths. Single machine, macOS APFS. The planned in-repo test `repackspike_test.go` was not written (spike only, no product changes).
