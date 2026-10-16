package s4lock

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func fixture(t *testing.T) string {
	d := t.TempDir()
	cg(t, d, "init", "-q", "-b", "master")
	for _, f := range []string{"k.txt", "m.txt", "d.txt", "r.txt", "g1.txt", "g2.txt"} {
		write(t, d, f, "v1 "+f+"\n")
	}
	for i := 0; i < 20; i++ {
		write(t, d, fmt.Sprintf("sub/o%02d.txt", i), "o\n")
	}
	cg(t, d, "add", "-A")
	cg(t, d, "commit", "-qm", "c1")
	cg(t, d, "branch", "other")
	write(t, d, "c.txt", "c\n")
	cg(t, d, "add", "c.txt")
	cg(t, d, "commit", "-qm", "c2")
	write(t, d, "m.txt", "v2 modified\n")
	os.Remove(filepath.Join(d, "d.txt"))
	write(t, d, "u.txt", "untracked\n")
	cg(t, d, "mv", "r.txt", "r2.txt")
	return d
}

type op struct {
	name string
	gg   func(sc *Scope) error
	cli  [][]string
	// probe-only: worktree-writing ops that stay CLI-routed in production
	probe bool
}

func ops() []op {
	// fresh options per call: CommitOptions.Validate mutates Parents (shared reuse => "object not found")
	cm := func() *git.CommitOptions { return &git.CommitOptions{Author: sig(), Committer: sig()} }
	cmA := func() *git.CommitOptions { return &git.CommitOptions{Author: sig(), Committer: sig(), All: true} }
	cmAA := func() *git.CommitOptions {
		return &git.CommitOptions{Author: sig(), Committer: sig(), All: true, Amend: true}
	}
	cmAm := func() *git.CommitOptions { return &git.CommitOptions{Author: sig(), Committer: sig(), Amend: true} }
	return []op{
		{name: "Commit(All)", gg: func(sc *Scope) error { _, e := sc.WT.Commit("x", cmA()); return e },
			cli: [][]string{{"commit", "-qa", "-m", "x"}}},
		{name: "Commit(All+Amend)", gg: func(sc *Scope) error { _, e := sc.WT.Commit("x", cmAA()); return e },
			cli: [][]string{{"commit", "-qa", "--amend", "-m", "x"}}},
		{name: "Add then Commit(Amend)", gg: func(sc *Scope) error {
			if _, e := sc.WT.Add("u.txt"); e != nil {
				return e
			}
			_, e := sc.WT.Commit("x", cmAm())
			return e
		}, cli: [][]string{{"add", "u.txt"}, {"commit", "-q", "--amend", "-m", "x"}}},
		{name: "Add(m.txt) then Commit", gg: func(sc *Scope) error {
			if _, e := sc.WT.Add("m.txt"); e != nil {
				return e
			}
			_, e := sc.WT.Commit("x", cm())
			return e
		}, cli: [][]string{{"add", "m.txt"}, {"commit", "-q", "-m", "x"}}},
		{name: "Remove then Status", gg: func(sc *Scope) error {
			if _, e := sc.WT.Remove("k.txt"); e != nil {
				return e
			}
			st, e := sc.WT.Status()
			if e != nil {
				return e
			}
			if st.File("k.txt").Staging != git.Deleted {
				return fmt.Errorf("in-scope Status did not see staged delete: %v", st.File("k.txt"))
			}
			return nil
		}, cli: [][]string{{"rm", "-q", "k.txt"}}, probe: true},
		{name: "RemoveGlob then Commit", gg: func(sc *Scope) error {
			if e := sc.WT.RemoveGlob("g*.txt"); e != nil {
				return e
			}
			_, e := sc.WT.Commit("x", cm())
			return e
		}, cli: [][]string{{"rm", "-q", "g1.txt", "g2.txt"}, {"commit", "-q", "-m", "x"}}, probe: true},
		{name: "Move then Status", gg: func(sc *Scope) error {
			if _, e := sc.WT.Move("k.txt", "k2.txt"); e != nil {
				return e
			}
			st, e := sc.WT.Status()
			if e != nil {
				return e
			}
			if st.File("k2.txt").Staging != git.Renamed && st.File("k2.txt").Staging != git.Added {
				return fmt.Errorf("in-scope Status did not see move: %v", st.File("k2.txt"))
			}
			return nil
		}, cli: [][]string{{"mv", "k.txt", "k2.txt"}}, probe: true},
		{name: "Restore(Staged r2.txt)", gg: func(sc *Scope) error {
			return sc.WT.Restore(&git.RestoreOptions{Staged: true, Files: []string{"r2.txt"}})
		}, cli: [][]string{{"restore", "--staged", "r2.txt"}}},
		{name: "Reset(Mixed)", gg: func(sc *Scope) error {
			return sc.WT.Reset(&git.ResetOptions{Mode: git.MixedReset})
		}, cli: [][]string{{"reset", "-q"}}},
		{name: "Reset(Soft HEAD~1)", gg: func(sc *Scope) error {
			return sc.WT.Reset(&git.ResetOptions{Mode: git.SoftReset, Commit: plumbing.ZeroHash}) // replaced below
		}, cli: [][]string{{"reset", "-q", "--soft", "HEAD"}}},
		{name: "Reset(Hard)", gg: func(sc *Scope) error {
			return sc.WT.Reset(&git.ResetOptions{Mode: git.HardReset})
		}, cli: [][]string{{"reset", "-q", "--hard"}}, probe: true},
		{name: "Checkout(Branch other, Keep)", gg: func(sc *Scope) error {
			return sc.WT.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("other"), Keep: true})
		}, cli: [][]string{{"checkout", "-q", "other"}}, probe: true},
		{name: "Checkout(Hash c1, Keep)", gg: func(sc *Scope) error {
			r, _ := sc.Repo.ResolveRevision("other")
			return sc.WT.Checkout(&git.CheckoutOptions{Hash: *r, Keep: true})
		}, cli: [][]string{{"checkout", "-q", "--detach", "other"}}, probe: true},
		{name: "AddWithOptions(All)", gg: func(sc *Scope) error {
			return sc.WT.AddWithOptions(&git.AddOptions{All: true})
		}, cli: [][]string{{"add", "-A"}}},
	}
}

func snap(t *testing.T, d string) map[string]string {
	m := map[string]string{}
	m["tree"] = cg(t, d, "rev-parse", "HEAD^{tree}")
	m["ls-files-s"] = cg(t, d, "ls-files", "-s")
	_, _, _ = gitRun(d, "update-index", "--refresh")
	m["status"] = cg(t, d, "status", "--porcelain=v1", "--untracked-files=all")
	if o, _, err := gitRun(d, "symbolic-ref", "-q", "HEAD"); err == nil {
		m["head-ref"] = strings.TrimSpace(o)
	} else {
		m["head-ref"] = "DETACHED"
	}
	m["parents"] = cg(t, d, "rev-list", "--count", "HEAD")
	var files []string
	filepath.Walk(d, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() && fi.Name() == ".git" {
			if fi != nil && fi.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !fi.IsDir() {
			b, _ := os.ReadFile(p)
			rel, _ := filepath.Rel(d, p)
			files = append(files, rel+"="+string(b))
		}
		return nil
	})
	sort.Strings(files)
	m["worktree"] = strings.Join(files, "|")
	return m
}

func TestT7_ReadYourWritesMatrix(t *testing.T) {
	for _, o := range ops() {
		o := o
		t.Run(o.name, func(t *testing.T) {
			a, b := fixture(t), fixture(t)
			gg := o.gg
			if o.name == "Reset(Soft HEAD~1)" { // soft reset to HEAD itself, via explicit hash
				gg = func(sc *Scope) error {
					h, _ := sc.Repo.Head()
					return sc.WT.Reset(&git.ResetOptions{Mode: git.SoftReset, Commit: h.Hash()})
				}
			}
			if err := WithIndexLock(a, Options{}, gg); err != nil {
				t.Logf("RESULT %-32s go-git scope error: %v", o.name, err)
				t.Fail()
				return
			}
			noLocks(t, a)
			for _, c := range o.cli {
				cg(t, b, c...)
			}
			sa, sb := snap(t, a), snap(t, b)
			var diffs []string
			for k := range sb {
				if sa[k] != sb[k] {
					diffs = append(diffs, k)
				}
			}
			if _, e, err := gitRun(a, "fsck", "--no-dangling"); err != nil {
				t.Errorf("fsck: %s", e)
			}
			if len(diffs) == 0 {
				t.Logf("RESULT %-32s == CLI twin (tree, ls-files -s, status, HEAD, worktree)  probe-only=%v", o.name, o.probe)
			} else {
				for _, k := range diffs {
					t.Logf("DIFF[%s] %s\n  gogit: %q\n  cli:   %q", o.name, k, trunc(sa[k]), trunc(sb[k]))
				}
				t.Errorf("RESULT %-32s DIFFERS from CLI twin in %v", o.name, diffs)
			}
		})
	}
}

func trunc(s string) string {
	if len(s) > 400 {
		return s[:400] + "..."
	}
	return s
}

// Hazard demo: lock-but-no-read-your-writes drops auto-added changes from the commit tree.
func TestT8_StaleReadHazard(t *testing.T) {
	a, b := fixture(t), fixture(t)
	cmA := func() *git.CommitOptions { return &git.CommitOptions{Author: sig(), Committer: sig(), All: true} }
	if err := WithIndexLock(a, Options{NoReadYourWrites: true}, func(sc *Scope) error { _, e := sc.WT.Commit("x", cmA()); return e }); err != nil {
		t.Fatal(err)
	}
	cg(t, b, "commit", "-qa", "-m", "x")
	ta, tb := cg(t, a, "rev-parse", "HEAD^{tree}"), cg(t, b, "rev-parse", "HEAD^{tree}")
	t.Logf("T8 naive (no RYW) tree=%s cli tree=%s equal=%v", ta, tb, ta == tb)
	if ta == tb {
		t.Log("T8: hazard NOT reproduced (stale read did not matter)")
	}
}
