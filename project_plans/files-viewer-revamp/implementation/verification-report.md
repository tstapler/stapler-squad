# Verification report: files-viewer-revamp (Phase 6)

Scope: everything implemented so far (`main...HEAD`: 166 commits, 402 files). Owner-gated work (phone sitting, Gates G1-G3, Epics 2.2-2.4/2.6) is not implemented and not covered.
Final tree: `073730179`, clean, nothing pushed. Live :8543/:8444 service untouched.

## Verdict: PASS for the implemented scope, with open owner decisions and unverified phone-only checks

Fix loop used 4 of 5 iterations (iteration 1: 19 items; 2: A1-A6 + B1-B8; 3: 4 SUGGESTs; 4: toast deck over Back button).

## Layers

| Layer | Result |
|---|---|
| 1 Idioms (Go, React/TS, TS lib) | MUST FIX items fixed across iterations 1-2; re-review of iteration 2 found 0 MUST FIX, 4 SUGGEST, fixed in iteration 3 |
| 2 Architecture / refactor | Blockers fixed in iteration 1; `make lint` "0 issues" (repo thresholds gocyclo 25, gocognit 40, funlen 150/100, dupl 150) |
| 3 Correctness, tests, security, acceptance | See below |
| 4 UX / behaviour | Partial: Playwright E2E on isolated server plus emulated Pixel 7 exploration; no real device |

## Evidence (all run by the coordinator on the final tree unless noted)

- Full jest: 695 suites passed, 8035 tests passed, 2 skipped (run at `2b64bc115`; iteration 4 adds one test file, `src/components/sessions` re-run by the fix agent: 168 suites passed).
- Playwright chromium (search, viewer, tabs, a11y, shortcuts): 18 passed, 1 skipped (streaming spike, by design), at `2b64bc115`.
- Playwright mobile-chromium (files-mobile, files-large, files-a11y, files-tabs, files-search), `--workers=1 --retries=0`, final tree: 27 passed.
- Go `-race -short`: `./server/services` ok (fix agent, 279 s, at `2b64bc115`); analysis/range/stat tests `-count=5` ok (coordinator).
- `make files-viewer-gates` (race step skipped via `FILES_VIEWER_SKIP_RACE=1`): exit 0, jscpd 0.05%, line budget PASS.
- Security review: no CRITICAL/HIGH. Fixed since: gzip-inflated body bypass of the 1.5 MiB cap (both chains), raw symlink targets in listings, raw OS errors.

## Known gaps and open decisions

1. **Tablet layout rule** (container width puts 820x1180 and 1180x820 in the mobile layout; Story 1.5.1 says otherwise): open owner decision, recorded in plan Story 1.5.1.
2. **Axe scan gaps**: file tree, tab strip (`aria-required-children`, `aria-required-parent`, `nested-interactive`), `.cm-scroller` contrast/focusability, session chrome are scoped out and unfixed.
3. **Large-file badge** in search needs a `SearchMatch` size field; chunk CRC verified only for LF or all-CRLF valid UTF-8.
4. **`files:range-read` off**: the full adapter refuses 50 MB with `failed_precondition` (10 MiB limit); plan wording differs (ADR-036 deviations section).
5. **Proto-only commits** 638ddfa23 and 2bcf9be3f: squash for bisectability or leave.
6. **Not verified**: everything phone-only (18 rows in `manual-verification-checklist.md`), WebKit and Firefox (need sudo libs), the race wall-clock gate step in the final gate run, `assertNonTextContrast` for the dirty dot/diff glyph (needs gated Edit mode) and for a second theme palette, the jest "worker failed to exit gracefully" leak warning, a deterministic E2E for a pinned toast over the top bar (unit test covers the anchor only), StatFile metric test never seen red.
7. **Flake note**: one early `-race` run by the fix agent printed FAIL (unrelated "failed to save default config" warning); test name not captured, did not recur in 5+ later runs.
8. **Environment**: first iteration-2 mobile runs failed at global setup on a shared `ssq-e2e-tmp/srv` dir (seed UNIQUE error); passes with a fresh `TEST_SERVER_DIR`.

## Deferred follow-ups

`EditorEngine` layering (lib imports component types), dead `resolveAndValidatePath`, unguarded `ServeFileRaw` route, zIndex magic-number sweep, `OpenTabsBar`/`TabSwitcherSheet`/`SearchPanel` decomposition, unwired `BufferPrefetcher`, `sweepStale` per save, cancellable path lock, `LoadConfig` per `ReadFileRange`, `analysisCache.Close` never called, no AbortSignal on `fetchFileContent`, unbounded module cache, dead `link_target` proto field not reserved. Delete the spike pages after the phone sitting.
