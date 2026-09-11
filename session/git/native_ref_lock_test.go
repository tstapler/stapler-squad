package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// newRefRaceFixture builds a repo with a branch ref at origHash, plus two other commits
// (defHash, ghiHash) minted purely as distinct write-race targets for
// TestNativeRefWrite_UnprotectedRace_CanCorruptRef/TestNativeRefWrite_LockSentinel_NeverCorruptsAgainstRealGit.
// branchName is never checked out, so the race only ever touches its loose ref file
// directly, never the worktree/index.
func newRefRaceFixture(t *testing.T) (repoDir, branchName, origHash, defHash, ghiHash string) {
	t.Helper()
	repoDir = setupTestRepo(t)
	runRealGit(t, repoDir, "config", "user.email", "test@example.com")
	runRealGit(t, repoDir, "config", "user.name", "Test User")

	branchName = "race-branch"
	runRealGit(t, repoDir, "branch", branchName)
	origHash = strings.TrimSpace(runRealGit(t, repoDir, "rev-parse", branchName))

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "def.txt"), []byte("def\n"), 0o644))
	runRealGit(t, repoDir, "add", ".")
	runRealGit(t, repoDir, "commit", "-m", "def commit")
	defHash = strings.TrimSpace(runRealGit(t, repoDir, "rev-parse", "HEAD"))

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "ghi.txt"), []byte("ghi\n"), 0o644))
	runRealGit(t, repoDir, "add", ".")
	runRealGit(t, repoDir, "commit", "-m", "ghi commit")
	ghiHash = strings.TrimSpace(runRealGit(t, repoDir, "rev-parse", "HEAD"))

	// defHash/ghiHash remain valid loose objects after this reset -- only main's branch
	// tip moves back, race-branch (created before either commit) is untouched.
	runRealGit(t, repoDir, "reset", "--hard", origHash)

	return repoDir, branchName, origHash, defHash, ghiHash
}

// pollRefForTorn spins reading refPath until stop is set, flagging sawCorruption if any
// read observes content that isn't exactly one of valid's keys (each "<hash>\n"). A
// concurrent unprotected writer's open(O_TRUNC)-then-Write() is not atomic (Task 2.5.3b's
// doc comment on writeRefWithLockSentinel explains why), so a reader polling fast enough
// can catch the file mid-write, empty or otherwise malformed -- exactly the "corrupted"
// state both acceptance criteria are stated in terms of.
func pollRefForTorn(refPath string, valid map[string]bool, stop, sawCorruption *int32) {
	for atomic.LoadInt32(stop) == 0 {
		b, err := os.ReadFile(refPath)
		if err != nil {
			continue
		}
		if !valid[string(b)] {
			atomic.StoreInt32(sawCorruption, 1)
		}
	}
}

// TestNativeRefWrite_UnprotectedRace_CanCorruptRef is Story 2.5.3's Task 2.5.3a: a
// deliberate demonstration that go-git's bare SetReference, racing a concurrent real `git
// update-ref` subprocess against the same ref, is unsafe -- proving pitfalls.md §1.4's
// finding (go-git's ref writes take an advisory flock a real git process never
// participates in, since git's own protocol is lockfile-presence, not flock) is a real,
// reproducible risk rather than a theoretical one. This test is expected to demonstrate
// the failure mode, not to "pass clean" in the sense of never observing it -- a green
// assertion here means the risk was reproduced. Do not "fix" the underlying race to make
// this test's mechanics disappear; TestNativeRefWrite_LockSentinel_NeverCorruptsAgainstRealGit
// is the actual fix's regression test.
func TestNativeRefWrite_UnprotectedRace_CanCorruptRef(t *testing.T) {
	const trials = 50
	corruptedTrials := 0

	for i := 0; i < trials; i++ {
		repoDir, branchName, origHash, defHash, ghiHash := newRefRaceFixture(t)
		refPath := filepath.Join(repoDir, ".git", "refs", "heads", branchName)
		valid := map[string]bool{
			origHash + "\n": true,
			defHash + "\n":  true,
			ghiHash + "\n":  true,
		}

		repo, err := OpenRepo(repoDir)
		require.NoError(t, err)

		var stop, sawCorruption int32
		var wg sync.WaitGroup
		wg.Add(3)
		start := make(chan struct{})
		go func() {
			defer wg.Done()
			<-start
			// The unprotected mechanism under test: go-git's own bare SetReference,
			// with no lock-sentinel protection at all.
			ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branchName), plumbing.NewHash(defHash))
			_ = repo.Storer.SetReference(ref)
		}()
		go func() {
			defer wg.Done()
			<-start
			cmd := safeexec.CommandContext(context.Background(), "git", "update-ref", "refs/heads/"+branchName, ghiHash)
			cmd.Dir = repoDir
			_ = cmd.Run()
			atomic.StoreInt32(&stop, 1)
		}()
		go func() {
			defer wg.Done()
			<-start
			pollRefForTorn(refPath, valid, &stop, &sawCorruption)
		}()
		close(start)
		wg.Wait()

		if atomic.LoadInt32(&sawCorruption) == 1 {
			corruptedTrials++
		}
	}

	assert.Positive(t, corruptedTrials,
		"expected at least one of %d trials to observe a torn/corrupted ref read during the unprotected race (see writeRefWithLockSentinel's doc comment in native_merge.go for the mechanism)", trials)
}

// TestNativeRefWrite_LockSentinel_NeverCorruptsAgainstRealGit is Story 2.5.3's Task
// 2.5.3c: the same 50-trial race as TestNativeRefWrite_UnprotectedRace_CanCorruptRef, but
// with the native side going through writeRefWithLockSentinel instead of go-git's bare
// SetReference. Real git's own `git update-ref` already uses the identical
// "<ref>.lock"-presence-then-rename protocol (confirmed via strace against git 2.53.0),
// so the two writers now collide on the same lockfile discipline real git uses against
// itself, never on the target ref file directly.
func TestNativeRefWrite_LockSentinel_NeverCorruptsAgainstRealGit(t *testing.T) {
	const trials = 50

	for i := 0; i < trials; i++ {
		repoDir, branchName, origHash, defHash, ghiHash := newRefRaceFixture(t)
		refPath := filepath.Join(repoDir, ".git", "refs", "heads", branchName)
		valid := map[string]bool{
			origHash + "\n": true,
			defHash + "\n":  true,
			ghiHash + "\n":  true,
		}

		var stop, sawCorruption int32
		var wg sync.WaitGroup
		wg.Add(3)
		start := make(chan struct{})
		go func() {
			defer wg.Done()
			<-start
			// A losing writer here returns an error (O_EXCL collision on the shared
			// ".lock" filename) rather than silently losing its write -- see the
			// deterministic sub-test below for a direct, non-timing-dependent proof of
			// that specific behavior.
			_ = writeRefWithLockSentinel(refPath, plumbing.NewHash(defHash))
		}()
		go func() {
			defer wg.Done()
			<-start
			cmd := safeexec.CommandContext(context.Background(), "git", "update-ref", "refs/heads/"+branchName, ghiHash)
			cmd.Dir = repoDir
			_ = cmd.Run()
			atomic.StoreInt32(&stop, 1)
		}()
		go func() {
			defer wg.Done()
			<-start
			pollRefForTorn(refPath, valid, &stop, &sawCorruption)
		}()
		close(start)
		wg.Wait()

		assert.Zero(t, atomic.LoadInt32(&sawCorruption), "trial %d: observed a torn/corrupted ref read even with writeRefWithLockSentinel in place", i)

		finalBytes, err := os.ReadFile(refPath)
		require.NoError(t, err)
		assert.True(t, valid[string(finalBytes)], "trial %d: final ref content %q must be exactly one of the two attempted values, never a third value", i, string(finalBytes))
	}

	t.Run("losing_writer_gets_error_not_silent_data_loss", func(t *testing.T) {
		repoDir, branchName, _, defHash, _ := newRefRaceFixture(t)
		refPath := filepath.Join(repoDir, ".git", "refs", "heads", branchName)
		lockPath := refPath + ".lock"

		f, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.Remove(lockPath) })
		t.Cleanup(func() { _ = f.Close() })

		beforeContent, err := os.ReadFile(refPath)
		require.NoError(t, err)

		err = writeRefWithLockSentinel(refPath, plumbing.NewHash(defHash))
		require.Error(t, err, "writeRefWithLockSentinel must fail, not silently clobber, when the lock sentinel already exists")

		afterContent, err := os.ReadFile(refPath)
		require.NoError(t, err)
		assert.Equal(t, string(beforeContent), string(afterContent), "a losing writer's CAS-style failure must leave refPath completely untouched")
	})
}
