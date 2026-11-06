package unfinished

import (
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

// FlagSpec describes one rollback-unit flag: its config key, the A/B trigger
// that flips its default (Story 5.2.1), and the date by which the flag and its
// legacy branch must be removed (ADR-008).
type FlagSpec struct {
	Name string
	// Trigger is the A/B trigger id (T1-T4) that flips this flag's default.
	Trigger string
	// RemoveBy is an ISO date (YYYY-MM-DD); flags_test.go fails after it.
	// Extending it needs a one-line justification in the diff.
	RemoveBy string
}

// FlagSpecs lists every flag. Dirty-watch Tier B and best_merge_base have no
// flag: Tier B is skipped and paint-down merge base is the default walk.
var FlagSpecs = []FlagSpec{
	{Name: "shared_commit_state", Trigger: "T3", RemoveBy: "2027-02-13"},
	{Name: "diff_counter", Trigger: "T1", RemoveBy: "2027-02-13"},
	{Name: "scan_skip", Trigger: "T4", RemoveBy: "2027-02-13"},
	{Name: "watch_worktree_gitdirs", Trigger: "T2", RemoveBy: "2027-02-13"},
}

// Flags is the normalized, immutable flag snapshot call sites read.
type Flags struct {
	SharedCommitState    bool
	DiffCounter          bool
	ScanSkip             bool
	WatchWorktreeGitdirs bool
	// ScanSkipPrelude gates the ref-snapshot/signature/index-cache sub-features
	// of scan_skip; they need shared_commit_state.
	ScanSkipPrelude bool
	// ScanSkipDirtyGen gates skips that read DirtyGen; they need
	// watch_worktree_gitdirs.
	ScanSkipDirtyGen bool
}

// normalizeFlags forces dependents off when a dependency is off and returns
// one warning per violated dependency.
func normalizeFlags(c config.UnfinishedCaches) (Flags, []string) {
	f := Flags{
		SharedCommitState:    c.SharedCommitState,
		DiffCounter:          c.DiffCounter,
		ScanSkip:             c.ScanSkip,
		WatchWorktreeGitdirs: c.WatchWorktreeGitdirs,
		ScanSkipPrelude:      c.ScanSkip && c.SharedCommitState,
		ScanSkipDirtyGen:     c.ScanSkip && c.WatchWorktreeGitdirs,
	}
	var warns []string
	if c.ScanSkip && !c.SharedCommitState {
		warns = append(warns, "unfinished: scan_skip prelude sub-features disabled, shared_commit_state is off")
	}
	if c.ScanSkip && !c.WatchWorktreeGitdirs {
		warns = append(warns, "unfinished: scan_skip DirtyGen sub-features disabled, watch_worktree_gitdirs is off")
	}
	return f, warns
}

// FlagsFromConfig normalizes cfg into the immutable Flags a Scanner is built
// with (WithFlags), logging one warning per violated dependency. Flags are
// restart-only: the server calls this once at startup and later config edits
// never reach a running Scanner.
func FlagsFromConfig(cfg config.UnfinishedCaches) Flags {
	f, warns := normalizeFlags(cfg)
	for _, w := range warns {
		log.Warn(w)
	}
	return f
}
