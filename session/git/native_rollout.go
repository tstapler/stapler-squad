package git

// useNativeWorktree resolves whether GitWorktree operations for sessionName should
// dispatch to Phase 2's native go-git implementation instead of the legacy subprocess
// implementation (plan.md's Domain Glossary). Every native/legacy dispatch wrapper in
// this package (setupNewWorktree, unlockWorktree, and their Epic 2.2-2.4 siblings) calls
// this exact function, so Epic 4.1 only needs to replace this var's body — never a call
// site — once it lands.
//
// TODO(Phase 4 Epic 4.1): wire to real config.GetNativeWorktreeSessionOverride /
// config.EffectiveNativeWorktreeEnabled once that epic lands (plan.md's Task 2.1.3a
// sequencing note — Epic 4.1 hasn't landed yet, so no such config exists to wire to).
// Until then this is a var holding a func literal (mirroring session/instance_workspace.go's
// timeNow seam), not a plain func declaration, specifically so tests can swap it directly
// to exercise the "native flag on" dispatch path without a real config-backed override.
var useNativeWorktree = func(sessionName string) bool {
	return false
}

// useNativeMerge resolves whether MergeMainIntoWorktree for worktreePath should dispatch
// to Phase 3's native go-git merge pipeline instead of the legacy subprocess
// implementation (Epic 3.4). Per ADR-002, this flag is keyed by worktreePath, not
// sessionName: MergeMainIntoWorktree has no session-name parameter to key an override off
// of without either a caller signature change or a new session/git-package reverse
// lookup, both rejected in ADR-002's Alternatives Considered.
//
// TODO(Phase 4 Epic 4.1): wire to real config.GetNativeMergeWorktreeOverride /
// config.EffectiveNativeMergeEnabled once that epic lands — no such config exists yet
// (same sequencing note as useNativeWorktree above). Until then this is a var holding a
// func literal, so tests can swap it directly to exercise the "native flag on" dispatch
// path without a real config-backed override.
var useNativeMerge = func(worktreePath string) bool {
	return false
}
