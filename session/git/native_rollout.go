package git

import "github.com/tstapler/stapler-squad/config"

// useNativeWorktree resolves whether GitWorktree operations for sessionName should
// dispatch to Phase 2's native go-git implementation instead of the legacy subprocess
// implementation (plan.md's Domain Glossary). Every native/legacy dispatch wrapper in
// this package (setupNewWorktree, unlockWorktree, and their Epic 2.2-2.4 siblings) calls
// this exact function. A session override takes precedence over the global default
// (ADR-002); config is loaded fresh on every call, matching EffectiveStreamHubEnabled's
// existing precedent and required by the flag-flip-mid-burst analysis in
// research/architecture.md §5.
//
// This is a var holding a func literal (mirroring session/instance_workspace.go's
// timeNow seam), not a plain func declaration, specifically so tests can swap it directly
// to exercise the "native flag on" dispatch path without a real config-backed override.
var useNativeWorktree = func(sessionName string) bool {
	cfg := config.LoadConfig()
	if v, ok := cfg.GetNativeWorktreeSessionOverride(sessionName); ok {
		return v
	}
	return config.EffectiveNativeWorktreeEnabled(cfg)
}

// useNativeMerge resolves whether MergeMainIntoWorktree for worktreePath should dispatch
// to Phase 3's native go-git merge pipeline instead of the legacy subprocess
// implementation (Epic 3.4). Per ADR-002, this flag is keyed by worktreePath, not
// sessionName: MergeMainIntoWorktree has no session-name parameter to key an override off
// of without either a caller signature change or a new session/git-package reverse
// lookup, both rejected in ADR-002's Alternatives Considered.
//
// This is a var holding a func literal, so tests can swap it directly to exercise the
// "native flag on" dispatch path without a real config-backed override.
var useNativeMerge = func(worktreePath string) bool {
	cfg := config.LoadConfig()
	if v, ok := cfg.GetNativeMergeWorktreeOverride(worktreePath); ok {
		return v
	}
	return config.EffectiveNativeMergeEnabled(cfg)
}
