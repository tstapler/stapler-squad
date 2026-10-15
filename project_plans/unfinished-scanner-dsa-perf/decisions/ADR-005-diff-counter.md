# ADR-005: Shortstat counter — oracle-first; in-package Myers on interned line ids is the leading candidate

**Status**: Proposed, finalized by Story 3.2.1 results · **Plan**: Epic 3.2

## Context
`lcsLength` (`G:1828`) is O(m·k), capped by `maxLCSLines` (`G:1824`), and only the LCS length is needed. Research conflicts: build-vs-buy recommends `sergi/go-diff` (already `go.mod:42`); stack research shows its half-match shortcut is active under the fork's 1 h timeout (`diffmatchpatch/diff.go:172,515`) and can return non-minimal scripts, and its rune/Diff allocations are heavy. Exact parity is required: `Insertions = len(new)-LCS`, `Deletions = len(old)-LCS`.

## Decision
1. Build the oracle first (Story 0.1.3b): `lcsLength` retained as a test-only reference, property tests on >= 10,000 seeded pairs.
2. Evaluate two candidates against it (Story 3.2.1): (a) in-package greedy Myers over `[]LineID` with prefix/suffix trimming; (b) `diffmatchpatch.DiffMainRunes` with `DiffTimeout=0`, never calling `DiffCleanupSemantic`.
3. Decision rule: exact parity required (zero mismatches), then lowest `B/op`. Expected winner (a); if (a) fails the property test after repair, use (b). Not a fork patch (AGENTS.md preference order: algorithm in package first).
4. `maxLCSLines` semantics preserved in this project (cap and approximation unchanged); exactness above the cap is a flagged follow-up, not part of this work.

## Consequences
Heuristic algorithms (patience/histogram/Myers with cutoff) are excluded because they are not parity-safe. Per-file memo (Story 3.2.3, key `{path, size, mtimeNs, ctimeNs, inode, headBlob}`; disabled where ctime is unavailable) is independent of this choice. Both ship under the `diff_counter` rollback unit (ADR-008).
