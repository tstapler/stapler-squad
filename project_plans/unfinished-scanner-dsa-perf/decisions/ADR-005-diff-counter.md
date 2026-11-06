# ADR-005: Shortstat counter — oracle-first; in-package Myers is the chosen candidate

**Status**: Accepted 2026-10-16 (Story 3.2.1 spike): candidate (a); call sites not yet swapped (Story 3.2.2) · **Plan**: Epic 3.2

## Context
`lcsLength` (`G:1828`) is O(m·k), capped by `maxLCSLines` (`G:1824`), and only the LCS length is needed. Research conflicts: build-vs-buy recommends `sergi/go-diff` (already `go.mod:42`); stack research shows its half-match shortcut is active under the fork's 1 h timeout (`diffmatchpatch/diff.go:172,515`) and can return non-minimal scripts, and its rune/Diff allocations are heavy. Exact parity is required: `Insertions = len(new)-LCS`, `Deletions = len(old)-LCS`.

## Decision
1. Build the oracle first (Story 0.1.3b): `lcsLength` retained as a test-only reference, property tests on >= 10,000 seeded pairs.
2. Evaluate two candidates against it (Story 3.2.1): (a) in-package greedy Myers over `[]LineID` with prefix/suffix trimming; (b) `diffmatchpatch.DiffMainRunes` with `DiffTimeout=0`, never calling `DiffCleanupSemantic`.
3. Decision rule: exact parity required (zero mismatches), then lowest `B/op`. Expected winner (a); if (a) fails the property test after repair, use (b). Not a fork patch (AGENTS.md preference order: algorithm in package first).
4. `maxLCSLines` semantics preserved in this project (cap and approximation unchanged); exactness above the cap is a flagged follow-up, not part of this work.

## Consequences
Heuristic algorithms (patience/histogram/Myers with cutoff) are excluded because they are not parity-safe. Per-file memo (Story 3.2.3, key `{path, size, mtimeNs, ctimeNs, inode, headBlob}`; disabled where ctime is unavailable) is independent of this choice. Both ship under the `diff_counter` rollback unit (ADR-008).

## Spike result (Story 3.2.1, 2026-10-16)

**Decision: candidate (a), `MyersEditCounts` (`session/unfinished/myers.go`).** Oracle: `countsOracle` over the retained `lcsLength` (`diffcounts_oracle_test.go`).

| Check | lcsLength | (a) Myers | (b) diffmatchpatch (`DiffTimeout=0`) |
|---|---|---|---|
| Mismatches vs oracle, 10,000 seeded pairs (`TestDiffCandidates_ShouldReportMismatchCountsVsOracle_When10000PairsEach`) | 0 (is the oracle) | **0** | 0 |
| `B/op`, 2,000 lines / 15 changes (`TestDiffCandidates_ShouldReportAllocationsVsLcsLength_When2000Lines15Changes`, ReadMemStats deltas) | 32,768 | **0** (0.0% of lcsLength) | 511,184 (1,560%) |
| `allocs/op`, same fixture | 2 | **0** (0.0%) | 453 |
| ns/op, same fixture (advisory `BenchmarkDiffCandidates`, Apple M5 Max, 20x) | 74.8 ms | **0.20 ms** | 4.4 ms |
| ns/op, 2,000 disjoint lines (worst case, advisory) | 7.9 ms | 37.7 ms | 17.5 ms |

- Both candidates are exact on the corpus; the decision rule (exact parity, then lowest `B/op`) picks (a). (b) is exact only because `DiffTimeout=0` disables the half-match shortcut, and it allocates two to three orders of magnitude more.
- **G5 is met with margin**: `TestMyersCounter_ShouldUseAtMost25PctBytesAndAllocsOfLcsLength_When2000Lines15Changes` asserts `B/op` and `allocs/op` <= 25% of `lcsLength` via `runtime.ReadMemStats` deltas inside a `Test*` (skipped under `-race`, where `sync.Pool` drops Puts); its negative twin `TestG5Gate_ShouldFail_WhenLcsLengthIsWiredAsNew` proves the gate fails when `lcsLength` is wired in as the new implementation.
- Shape of (a): trims common prefix/suffix, computes only the edit distance D (LCS = (n+m-D)/2, so no trace is stored), compares lines as strings directly (no interning table: a map would allocate and the fixture needs 0 allocs to meet the 25% ceiling on a 2-allocs baseline), and reuses its V array through a `sync.Pool` (per-P, lock-free fast path; no mutex). Steady state is zero heap allocations.
- **Known worst case**: O((n+m)·D) is worse than `lcsLength`'s O(n·m) when almost no lines match (2,000 disjoint lines: 37.7 ms vs 7.9 ms, 4.8x). Real shortstat inputs are small edits to tracked files, so this is accepted; `maxLCSLines` still bounds it at the call site. A hybrid that falls back to the DP above a D threshold stays exact and is a flagged follow-up, not part of this decision.
- Not done in this spike (Story 3.2.2): swapping `lcsLength`/`LinesDiff` call sites, the `[]byte` twin (`TestMyersBytes_ShouldEqualLcsLengthBytes_WhenSeededPairs`), and the `maxLCSLines` cap check inside the new path (`MyersEditCounts` has no cap).
