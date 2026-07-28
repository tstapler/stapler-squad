package unfinished

// White-box test proving the "unfinished:mmap-index" feature flag actually
// controls gogitstore's mmap loader dynamically — no process restart, via
// the same persisted config the Settings UI's feature-flag toggle writes to
// (server/services/feature_flag_service.go, config.SetFeatureFlag). See
// gogit_vcs_reader.go's syncMmapIndexFlag and
// session/unfinished/design/mmap-activation-runbook.md.

import (
	"testing"

	"github.com/tstapler/stapler-squad/config"
)

// TestOpenRepoEntry_MmapIndexFlag_TogglesLiveWithoutRestart proves the flag
// is read fresh on each openRepoEntry call, not fixed once at
// GoGitVCSReader construction — flipping the persisted flag between two
// opens (of two different repos, so each actually creates a new
// SharedObjectStore rather than hitting the "not retroactive" case
// documented on Registry.UseMmapIndex) must change gogitstore's behavior
// without recreating the GoGitVCSReader or its Registry.
func TestOpenRepoEntry_MmapIndexFlag_TogglesLiveWithoutRestart(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	repoA := initRepoInternal(t)
	repoB := initRepoInternal(t)

	g := &GoGitVCSReader{}
	reg := g.gogitstoreRegistry()

	// Flag starts unset (absent key defaults to false per
	// config.GetFeatureFlag's doc comment).
	if _, err := g.openRepoEntry(repoA); err != nil {
		t.Fatalf("openRepoEntry(repoA): %v", err)
	}
	if reg.UseMmapIndex {
		t.Error("Registry.UseMmapIndex = true before the feature flag was ever set; want false (default)")
	}

	// Flip the flag via the exact same config API the Settings UI's
	// UpdateFeatureFlag RPC uses.
	cfg := config.LoadConfig()
	if err := cfg.SetFeatureFlag(mmapIndexFeatureFlag, true); err != nil {
		t.Fatalf("SetFeatureFlag: %v", err)
	}

	// Same GoGitVCSReader, same Registry — no restart, no new construction.
	if _, err := g.openRepoEntry(repoB); err != nil {
		t.Fatalf("openRepoEntry(repoB): %v", err)
	}
	if !reg.UseMmapIndex {
		t.Error("Registry.UseMmapIndex = false after SetFeatureFlag(mmapIndexFeatureFlag, true) and a subsequent openRepoEntry call; want true — the flag should apply live")
	}

	// Flip back off and confirm a third repo picks that up too.
	if err := cfg.SetFeatureFlag(mmapIndexFeatureFlag, false); err != nil {
		t.Fatalf("SetFeatureFlag: %v", err)
	}
	repoC := initRepoInternal(t)
	if _, err := g.openRepoEntry(repoC); err != nil {
		t.Fatalf("openRepoEntry(repoC): %v", err)
	}
	if reg.UseMmapIndex {
		t.Error("Registry.UseMmapIndex = true after SetFeatureFlag(mmapIndexFeatureFlag, false); want false — the flag should apply live in both directions")
	}
}

// TestSyncMmapIndexFlag_UnknownFlagDefaultsFalse guards against a typo in
// mmapIndexFeatureFlag ever silently making this default to enabled — an
// absent/misspelled flag name must resolve to false, matching
// config.GetFeatureFlag's documented "absent key returns false" contract.
func TestSyncMmapIndexFlag_UnknownFlagDefaultsFalse(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	g := &GoGitVCSReader{}
	reg := g.gogitstoreRegistry()
	syncMmapIndexFlag(reg)
	if reg.UseMmapIndex {
		t.Error("syncMmapIndexFlag set UseMmapIndex = true with no feature flag ever persisted; want false")
	}
}
