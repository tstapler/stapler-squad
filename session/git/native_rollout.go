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
