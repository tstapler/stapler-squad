// S7 stress harness: CLI writers race go-git readers on a linked worktree.
//
//	go run ./stress -common=true -dur=60s -pack=true -gc=true
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func sh(dir string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@a", "GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@a", "GIT_CONFIG_NOSYSTEM=1")
	out, err := c.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

var (
	histMu sync.Mutex
	hist   []string // confirmed tips of branch "wt", in order
)

func histLen() int { histMu.Lock(); defer histMu.Unlock(); return len(hist) }

func main() {
	common := flag.Bool("common", true, "EnableDotGitCommonDir")
	dur := flag.Duration("dur", 60*time.Second, "duration")
	pack := flag.Bool("pack", true, "pack-refs --all --prune racer")
	gc := flag.Bool("gc", true, "gc --prune=now / repack -ad racer")
	addrm := flag.Bool("addrm", true, "worktree add/remove racer")
	readers := flag.Int("readers", 4, "reader goroutines")
	fast := flag.Bool("fast", false, "writer B uses commit-tree + update-ref (higher rate, no index/HEAD-lock traffic)")
	flag.Parse()

	root, _ := os.MkdirTemp("", "s7stress")
	defer os.RemoveAll(root)
	mainDir := filepath.Join(root, "main")
	os.MkdirAll(mainDir, 0o755)
	sh(mainDir, "init", "-q", "-b", "main")
	sh(mainDir, "commit", "-q", "--allow-empty", "-m", "c1")
	wt := filepath.Join(root, "wt")
	if o, err := sh(mainDir, "worktree", "add", "-q", "-b", "wt", wt); err != nil {
		panic(o)
	}
	tip, _ := sh(wt, "rev-parse", "HEAD")
	hist = append(hist, tip)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	loop := func(f func(i int)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				f(i)
			}
		}()
	}
	var writes atomic.Int64
	// writer B: advance branch wt in the linked worktree (commit), also update-ref churn
	loop(func(i int) {
		if *fast {
			cur, _ := sh(wt, "rev-parse", "HEAD")
			tree, _ := sh(wt, "rev-parse", "HEAD^{tree}")
			nh, err := sh(wt, "commit-tree", tree, "-p", cur, "-m", fmt.Sprint("f", i))
			if err != nil {
				return
			}
			if _, err := sh(wt, "update-ref", "refs/heads/wt", nh, cur); err == nil {
				histMu.Lock()
				hist = append(hist, nh)
				histMu.Unlock()
				writes.Add(1)
			}
			return
		}
		if _, err := sh(wt, "commit", "-q", "--allow-empty", "-m", fmt.Sprint("w", i)); err == nil {
			h, _ := sh(wt, "rev-parse", "HEAD")
			histMu.Lock()
			hist = append(hist, h)
			histMu.Unlock()
			writes.Add(1)
		}
	})
	// writer A: commits on main in the primary worktree
	loop(func(i int) { sh(mainDir, "commit", "-q", "--allow-empty", "-m", fmt.Sprint("m", i)) })
	if *pack {
		loop(func(i int) { sh(mainDir, "pack-refs", "--all", "--prune"); time.Sleep(2 * time.Millisecond) })
	}
	if *gc {
		loop(func(i int) {
			if i%2 == 0 {
				sh(mainDir, "repack", "-adq")
			} else {
				sh(mainDir, "gc", "-q", "--prune=now")
			}
			time.Sleep(20 * time.Millisecond)
		})
	}
	if *addrm {
		loop(func(i int) {
			p := filepath.Join(root, fmt.Sprint("x", i))
			sh(mainDir, "worktree", "add", "-q", "-b", fmt.Sprint("x", i), p)
			sh(mainDir, "worktree", "remove", "--force", p)
			sh(mainDir, "branch", "-D", fmt.Sprint("x", i))
		})
	}

	var cliAgree, cliDiffer atomic.Int64
	var reads, errs, noobj, stale, wrongTarget atomic.Int64
	var mu sync.Mutex
	errBuckets := map[string]int{}
	var sample []string
	note := func(s string) {
		mu.Lock()
		if len(sample) < 8 {
			sample = append(sample, s)
		}
		mu.Unlock()
	}
	opts := &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: *common}
	for r := 0; r < *readers; r++ {
		loop(func(i int) {
			l1 := histLen()
			repo, err := git.PlainOpenWithOptions(wt, opts)
			if err != nil {
				errs.Add(1)
				mu.Lock()
				errBuckets["open: "+bucket(err)]++
				mu.Unlock()
				return
			}
			reads.Add(1)
			sym, serr := repo.Reference(plumbing.HEAD, false)
			if serr == nil && sym.Target() != "refs/heads/wt" {
				wrongTarget.Add(1)
				note("symbolic target " + sym.Target().String())
			}
			h, err := repo.Head()
			l2 := histLen()
			if err != nil {
				errs.Add(1)
				mu.Lock()
				errBuckets["Head: "+bucket(err)]++
				mu.Unlock()
				note("Head err: " + err.Error())
				return
			}
			hash := h.Hash()
			if _, cerr := repo.CommitObject(hash); cerr != nil {
				// distinguish truly absent from a pack swap racing the read: re-ask the CLI
				if o, _ := sh(wt, "cat-file", "-t", hash.String()); o == "commit" {
					mu.Lock()
					errBuckets["CommitObject failed but CLI sees object (transient repack race)"]++
					mu.Unlock()
					return
				}
				noobj.Add(1)
				cli, _ := sh(wt, "rev-parse", "HEAD")
				if cli == hash.String() {
					cliAgree.Add(1)
				} else {
					cliDiffer.Add(1)
				}
				note("valid-SHA-no-object " + hash.String() + " cliHEAD=" + cli + " err=" + cerr.Error())
				return
			}
			histMu.Lock()
			idx := -1
			for k := len(hist) - 1; k >= 0; k-- {
				if hist[k] == hash.String() {
					idx = k
					break
				}
			}
			histMu.Unlock()
			// valid window: confirmed index range [l1-1, l2] (one in-flight commit not yet recorded)
			if idx >= 0 && idx < l1-1 {
				stale.Add(1)
				note(fmt.Sprintf("stale: read idx %d, window [%d,%d]", idx, l1-1, l2))
			}
		})
	}
	time.Sleep(*dur)
	close(stop)
	wg.Wait()
	keys := make([]string, 0, len(errBuckets))
	for k := range errBuckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("common=%v dur=%v pack=%v gc=%v addrm=%v readers=%d\n", *common, *dur, *pack, *gc, *addrm, *readers)
	fmt.Printf("reads=%d wt_commits=%d class1_errors=%d class2_valid_sha_no_object=%d class3_stale_existing=%d wrong_symbolic_target=%d\n",
		reads.Load(), writes.Load(), errs.Load(), noobj.Load(), stale.Load(), wrongTarget.Load())
	fmt.Printf("class2 split: CLI rev-parse HEAD returned the same SHA (ref itself dangling)=%d, CLI returned a different SHA (go-git-only phantom)=%d\n", cliAgree.Load(), cliDiffer.Load())
	if o, _ := sh(wt, "fsck", "--no-dangling"); o != "" {
		fmt.Println("final fsck:", strings.ReplaceAll(o, "\n", " | "))
	}
	for _, k := range keys {
		fmt.Printf("  bucket %-70s %d\n", k, errBuckets[k])
	}
	for _, s := range sample {
		fmt.Println("  sample:", s)
	}
}

func bucket(err error) string {
	switch {
	case errors.Is(err, plumbing.ErrReferenceNotFound):
		return "ErrReferenceNotFound"
	default:
		s := err.Error()
		if len(s) > 80 {
			s = s[:80]
		}
		return s
	}
}
