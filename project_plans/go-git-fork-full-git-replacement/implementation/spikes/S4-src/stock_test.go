package s4lock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// Is "object not found" from RemoveGlob+Commit a scope artefact or stock go-git behaviour?
func TestT11_StockRemoveGlobCommit(t *testing.T) {
	d := fixture(t)
	r, _ := git.PlainOpen(d)
	w, _ := r.Worktree()
	if err := w.RemoveGlob("g*.txt"); err != nil {
		t.Fatal(err)
	}
	_, err := w.Commit("x", &git.CommitOptions{Author: sig(), Committer: sig()})
	t.Logf("T11 stock (no lock) RemoveGlob then Commit: err=%v", err)
}

// Attribute matrix divergences: do they occur with stock go-git and NO lock layer?
func TestT12_StockResetHardAndCheckout(t *testing.T) {
	d := fixture(t)
	r, _ := git.PlainOpen(d)
	w, _ := r.Worktree()
	err := w.Reset(&git.ResetOptions{Mode: git.HardReset})
	_, statErr := os.Stat(filepath.Join(d, "u.txt"))
	t.Logf("T12 stock Reset(Hard): err=%v untracked u.txt survives=%v (CLI keeps it)", err, statErr == nil)

	d = fixture(t)
	r, _ = git.PlainOpen(d)
	w, _ = r.Worktree()
	err = w.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("other"), Keep: true})
	o, _, _ := gitRun(d, "status", "--porcelain=v1")
	t.Logf("T12 stock Checkout(other,Keep): err=%v status=%q", err, strings.ReplaceAll(strings.TrimSpace(o), "\n", " | "))
}

// Deterministic contention: CLI add while a scope holds the lock must fail on index.lock (never write).
func TestT13_CLIBlockedWhileScopeHolds(t *testing.T) {
	d := newRepo(t, 50)
	write(t, d, "a.txt", "a")
	write(t, d, "b.txt", "b")
	var msg string
	var cliErr error
	err := WithIndexLock(d, Options{}, func(sc *Scope) error {
		_, e, er := gitRun(d, "add", "a.txt")
		msg, cliErr = e, er
		_, e2 := sc.WT.Add("b.txt")
		return e2
	})
	if err != nil {
		t.Fatal(err)
	}
	if cliErr == nil || !strings.Contains(msg, "index.lock") {
		t.Fatalf("CLI add should have failed on index.lock: err=%v msg=%q", cliErr, msg)
	}
	cg(t, d, "add", "a.txt") // lock released: CLI now succeeds
	fs := strings.Join(lsFiles(t, d), ",")
	if !strings.Contains(fs, "a.txt") || !strings.Contains(fs, "b.txt") {
		t.Fatal("missing entries")
	}
	noLocks(t, d)
	t.Logf("T13 CLI add inside held scope: exit=%v stderr(first line)=%q ; after release CLI add succeeded; both entries present", cliErr, strings.SplitN(msg, "\n", 2)[0])
}
