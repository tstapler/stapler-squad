# UX Research: fix-worktreeconfig-extension-compat

**Status: N/A — infrastructure bugfix only**

## Why UX Research Is Not Applicable

This fix is pure Go backend infrastructure. It corrects a go-git extension
compatibility issue in worktree config parsing. No user-facing surface is
added, changed, or removed; no UI components, copy, flows, or interactions
are touched.

## User-Visible Symptom (pre-fix)

Session creation fails when the target repository's `.git/worktrees/<name>/config`
contains extensions that the go-git library does not recognize. The failure
surfaces as an error log entry and a failed-to-create session state in the
UI — the user sees no session appear after the create action.

## User-Visible Recovery (post-fix)

Sessions create successfully for all affected repositories. No UI change is
visible; the fix is entirely in the error path that previously aborted
session creation.
