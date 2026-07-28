# Validation Plan: launchd-shell-sourcing

**Date**: 2026-07-25

## Happy Path Scenario

Given `scripts/install-service.sh` on `main` (commit `4eca0ed34`, with `baca1c7c2`
as an ancestor) and `docs/tasks/completed/system-service-autostart.md` containing
five existing `BUG-NNN` entries, when this session verifies criteria 0-2 by
inspection and inserts a sixth `BUG-006` entry documenting the deferred
`ANTHROPIC_API_KEY`/`GITHUB_TOKEN` env-inheritance gap, then all six acceptance
criteria have an explicit, evidenced disposition (pass or blocked) recorded via
`report_progress`/`/backlog/fail-N` — no criterion is silently skipped.

This item has zero new runtime code paths (no service, no API, no UI) — the
"test suite" for criteria 0-2 is direct inspection of already-shipped code, and
for criterion 5 is a structural/content review of one new Markdown block. There
are no unit/integration frameworks to invoke.

## Requirement → Check Mapping

| Requirement (plan.md Story) | Check | Type | Scenario |
|---|---|---|---|
| Story 1 / AC 0: no `.zshrc`/`.zprofile`/`/bin/zsh -c` refs | `grep -n "zshrc\|zprofile\|/bin/zsh -c" scripts/install-service.sh` | Inspection | Happy path — expect exit 1, no output |
| Story 1 / AC 1: macOS plist invokes binary directly, PATH via EnvironmentVariables | Read `install_macos()` (`scripts/install-service.sh:143-266`), confirm `ProgramArguments[0]` is `$bin_path`, `EnvironmentVariables` dict has `PATH` | Inspection | Happy path |
| Story 1 / AC 2: Linux unit invokes binary directly, PATH via Environment= | Read `install_linux()` (`scripts/install-service.sh:75-140`), confirm `ExecStart=$bin_path ...`, `Environment=PATH=$PATH` present | Inspection | Happy path |
| Story 1 (regression guard) | `grep -c "zshrc\|zprofile" scripts/install-service.sh` returns 0 even after the BUG-006 doc edit (doc edit touches a different file, so this must remain unchanged) | Inspection | Error/regression path — confirms the doc-only change didn't accidentally reintroduce shell sourcing |
| Story 2 / AC 5: BUG-006 entry documents the gap as deferred, not resolved | Re-read inserted block; confirm `**Status:** Deferred — not implemented this session` line present, `**Mitigation:**` heading NOT used (relabeled), no shell-rc-sourcing suggested as a fix (per architecture-review.md remediation, no `/bin/sh` wrapper listed either) | Inspection | Happy path |
| Story 2 / AC 5 (error path) | Confirm the entry does NOT claim `GITHUB_TOKEN`/`ANTHROPIC_API_KEY` are still inherited (would misreport current behavior) and does NOT overstate blanket breakage (per pitfalls.md conditional-impact framing) | Inspection | Error path — wording that would misrepresent severity |
| Story 3 / AC 3, 4: blocked criteria reported, not skipped | `get_backlog_item` after `/backlog/fail-3` and `/backlog/fail-4` shows explicit blocked status with reasoning for both indices | Inspection | Verifies no silent omission |

## UX Acceptance Tests

N/A — no user-facing surface. This is a backlog-administration and
documentation closeout for an already-shipped infra fix.

## Test Stack

- **Unit/Integration**: N/A — no code changes. All checks are `grep`/`Read`
  inspection, run manually and recorded as verification-notes evidence for
  `request_review`.
- **E2E / UX**: N/A.

## Coverage Targets and How to Measure

Not applicable in the line-coverage sense (no code changes). Coverage here
means "criterion has an explicit, evidenced disposition":

| Criterion | Disposition | Evidence |
|---|---|---|
| 0 (AC1) | Pass | grep exit 1, no output |
| 1 (AC2) | Pass | `install_macos()` read, lines cited above |
| 2 (AC3) | Pass | `install_linux()` read, lines cited above |
| 3 (AC4, archive) | Blocked | No `ArchiveBacklogItem`-equivalent MCP tool in work-role toolset (confirmed via `ToolSearch`) |
| 4 (AC5, cross-link) | Blocked | Same missing-tool gap as criterion 3; also depends on criterion 3's archival existing first |
| 5 (AC6, document gap) | Pass | New `BUG-006` entry in `docs/tasks/completed/system-service-autostart.md` |

6/6 criteria have an explicit disposition (4 pass, 2 explicitly blocked with
reasoning) — none silently skipped.
