# BUG-103: `TmuxSession.sanitizedName` is a bare `string`, not the `SessionName` type this file already defines [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-09-10, during `/quality:reflect-and-fix` on the em-dash/`&` tmux session name bug (see git history for `session/tmux/tmux.go`'s `nonSafeTmuxNameChar`).

## Problem Description

`session/tmux/tmux.go` defines a `SessionName` type (`NewSessionName`, unexported `value` field)
specifically so that "holding a `SessionName` proves the raw title has already been through
sanitization" — introduced after bug #162 (a session created as `staplersquad_CareerGrowth` but
addressed as `staplersquad_Career Growth`, a whitespace-drift version of the same failure class).

Despite that, `TmuxSession.sanitizedName` (the field every existence-check, kill, attach, and
capture-pane call actually uses) is declared as a plain `string`, not `SessionName`. Nothing at
compile time distinguishes "this string went through `toStaplerSquadTmuxNameWithPrefix`" from
"this string is raw user input" — the type system provides none of the guarantee `SessionName`
was built to provide.

This is exactly the gap that let the 2026-09-10 em-dash/`&` bug (see the "PR Code Review — ..."
/ "Research & Synthesize to Notes — ..." investigation) recur: the *sanitizer itself* has now
been hardened to an allowlist (`nonSafeTmuxNameChar`), but a future third construction site could
still assign an unsanitized string directly to `sanitizedName` and the compiler would never object.

## Fix

Convert `sanitizedName string` to `sanitizedName SessionName` in the `TmuxSession` struct.
`SessionName` already has a `String()` method, so most `%s`/`fmt.Stringer`-based usages (logging,
error messages) need no change. The two construction sites (`toStaplerSquadTmuxNameWithPrefix` via
`NewSessionName`, and `NewTmuxSessionFromExistingWithServerSocket`'s pre-validated exact-name path)
need updating, and the ~134 usage sites across `session/tmux/tmux.go` that pass `t.sanitizedName`
directly as a `string` (exec.Command args, map keys, `==` comparisons, `strings.*` calls) need
`.String()` added.

Scoped out of the 2026-09-10 session that found it: the mechanical size (134 call sites in the
single file most central to session creation) warranted its own reviewed change rather than
folding into an already-large diff. The allowlist sanitizer + `TestSanitizeName` regression cases
+ `FuzzToStaplerSquadTmuxName` (added in that session) already close the *specific* bug class;
this ticket is about closing the *structural* gap that let it (and bug #162 before it) happen at
all.

## Acceptance Criteria

- [ ] `sanitizedName` field type changed from `string` to `SessionName`
- [ ] Both construction sites produce a `SessionName` directly (no `string`-typed intermediate)
- [ ] All ~134 usage sites in `session/tmux/tmux.go` compile against the new type (`.String()`
      added wherever a bare `string` is required, e.g. `exec.Command` argv elements)
- [ ] `go build ./...` and `go test ./session/tmux/...` pass
- [ ] A new construction site that bypasses `NewSessionName`/`toStaplerSquadTmuxNameWithPrefix`
      (e.g. `sanitizedName: someRawTitle`) fails to compile, proving the gap is actually closed
