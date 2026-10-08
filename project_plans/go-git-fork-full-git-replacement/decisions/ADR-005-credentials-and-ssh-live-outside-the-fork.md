# ADR-005: Credential-helper client and HTTPS-to-SSH fallback are out-of-tree `transport.AuthMethod`s

**Status**: Proposed (validated by spike S5 / gate G5)
**Date**: 2026-10-08

## Context
- go-git has no `credential.helper` support (#1420 closed `not_planned` 2025-09-03; #490 closed, unimplemented). Auth is caller-supplied `transport.AuthMethod` (research/stack.md section 3, VERIFIED).
- SSH agent works in v5.19.2 (`transport/ssh/auth_method.go:185`); `~/.ssh/config` handling is narrower than OpenSSH (#509 open); `insteadOf` is not applied (#844 open).
- The "ssh-fallback" wrapper is a dotfiles convenience at `~/.local/bin/git` (research/build-vs-buy.md; `stapler-scripts/git-ssh-fallback`), not product behaviour. No product Go code references it.
- No product code uses go-git networking today (`git grep -E 'transport/(http|ssh)'` over non-test `.go` is empty); all fetch/push/clone are CLI (`session/repo_path.go`, `session/git/ops.go:36`, `session/vc/git_provider.go`).
- Recent credential-leak CVE on cross-host redirect (CVE-2026-41506; #2136 open) is in this exact path.

## Decision
- Implement the `git credential fill/approve/reject` protocol client in a repo package (`session/git/backend/gogit/credential`), exec'ing the configured helper binary directly (never `git credential`), with a Go-native macOS keychain path and `gh` token reuse tried first, and the configured helper binary as an **always-on fallback** (Revision 6, consistency C9: the requirements Constraints require the user's system credential helpers to keep working, so the fallback cannot be opt-in; plan Story 3.1.1 agrees). Host-scoped token lookup is injected as a `TokenSource` by `session/gitwiring`, which adapts `github.GetKeychainTokenForHost` (`github/keychain.go:131`); the credential package never imports `github`, because `go list -deps ./github` pulls in `config`, `executor/safeexec`, `session/git`, `session/tmux` and `session/lifecycle` (VERIFIED 2026-10-08) and would break the plan's Story 1.1.0 dependency check. Result is a `transport.AuthMethod`.
- HTTPS-to-SSH fallback is an application-level retry in the network backend, not a transport patch: on HTTPS auth failure, rewrite to the SSH URL and use agent auth. It reproduces the maintainer's wrapper only as an opt-in setting; default off because the wrapper is not product behaviour (open decision O-5).
- Install a custom HTTP client via `client.InstallProtocol` with an explicit `CheckRedirect` that refuses to forward credentials across hosts.
- `insteadOf` rewriting is applied by our own resolver before calling go-git.
- Nothing here is placed in the fork. The fork only receives a patch if S5 proves a public hook is missing.

## Consequences
- The network cohort ships without depending on fork patches; the fork stays thin.
- "Zero git spawn" holds for `git`, not for third-party credential helper binaries (ADR-003 carve-out).
- We own the redirect-credential policy and must keep it correct as upstream changes.
- ProxyCommand/ProxyJump/Match users are routed to the CLI by the capability preflight (ADR-006).

## Alternatives rejected
- **Patch credential-helper support into the fork**: a large greenfield piece in the exact code upstream is changing; contribute upstream later if it proves clean.
- **Shell to `git credential fill`**: spawns `git`, defeating the goal.
