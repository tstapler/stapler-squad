package s4lock

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
)

// T1: stock go-git Add vs CLI git add on the same index; count lost entries.
func TestT1_StockLostUpdate(t *testing.T) {
	lost, errsCLI := 0, 0
	const iters = 20
	for i := 0; i < iters; i++ {
		d := newRepo(t, 3000)
		write(t, d, "a.txt", "a")
		write(t, d, "b.txt", "b")
		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			if _, e, err := gitRun(d, "add", "a.txt"); err != nil {
				_ = e
				errsCLI++
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			r, _ := git.PlainOpen(d)
			w, _ := r.Worktree()
			w.Add("b.txt")
		}()
		close(start)
		wg.Wait()
		fs := strings.Join(lsFiles(t, d), "\n")
		if !strings.Contains(fs, "a.txt") || !strings.Contains(fs, "b.txt") {
			lost++
		}
	}
	t.Logf("T1 stock: iterations=%d lost-or-missing=%d cliLockErrors=%d", iters, lost, errsCLI)
}

// retryCLI runs `git add f` retrying while it reports index.lock; returns number of collisions.
func retryCLI(d, f string) (int, error) {
	c := 0
	for {
		_, e, err := gitRun(d, "add", f)
		if err == nil {
			return c, nil
		}
		if strings.Contains(e, "index.lock") {
			c++
			time.Sleep(time.Millisecond)
			continue
		}
		return c, fmt.Errorf("%v: %s", err, e)
	}
}

// T2: same race through WithIndexLock, 200 iterations on one repo => 400 new entries.
func TestT2_ScopedNoLostUpdate(t *testing.T) {
	d := newRepo(t, 200)
	base := len(lsFiles(t, d))
	var cliCollide, goCollide int
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		fa, fb := fmt.Sprintf("f%03d.txt", i), fmt.Sprintf("g%03d.txt", i)
		write(t, d, fa, "x")
		write(t, d, fb, "y")
		start := make(chan struct{})
		var cerr, gerr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			var c int
			c, cerr = retryCLI(d, fa)
			cliCollide += c
		}()
		go func() {
			defer wg.Done()
			<-start
			for {
				gerr = WithIndexLock(d, Options{}, func(sc *Scope) error { _, e := sc.WT.Add(fb); return e })
				if _, ok := gerr.(*ErrLocked); ok {
					goCollide++
					time.Sleep(time.Millisecond)
					continue
				}
				return
			}
		}()
		close(start)
		wg.Wait()
		if cerr != nil || gerr != nil {
			t.Fatalf("iter %d: cli=%v go=%v", i, cerr, gerr)
		}
	}
	got := len(lsFiles(t, d))
	t.Logf("T2: entries=%d (expected %d) cliLockCollisions=%d gogitErrLockedRetries=%d", got, base+400, cliCollide, goCollide)
	if got != base+400 {
		t.Fatalf("lost writes: %d != %d", got, base+400)
	}
	noLocks(t, d)
	if _, e, err := gitRun(d, "fsck", "--no-dangling"); err != nil {
		t.Fatalf("fsck: %s", e)
	}
}

func corruptMsg(s string) bool {
	for _, m := range []string{"corrupt", "bad index", "signature", "index file smaller", "unknown index", "fatal", "error:"} {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

func readerLoop(d string, stop <-chan struct{}, bad *[]string, n *int, mu *sync.Mutex) {
	cmds := [][]string{{"status", "--porcelain=v1"}, {"ls-files", "-s"}, {"add", "--", "reader-untracked.txt"}}
	i := 0
	for {
		select {
		case <-stop:
			return
		default:
		}
		args := cmds[i%len(cmds)]
		i++
		o, e, err := gitRun(d, args...)
		mu.Lock()
		*n++
		// lock contention for `git add` is the CLI doing its job, not corruption.
		if (err != nil || corruptMsg(e)) && !strings.Contains(e, "index.lock") {
			*bad = append(*bad, fmt.Sprintf("%v: %v: %s", args, err, e))
		}
		if args[0] == "ls-files" && err == nil && strings.Count(o, "\n") < 5000 {
			*bad = append(*bad, fmt.Sprintf("truncated ls-files: %d lines", strings.Count(o, "\n")))
		}
		mu.Unlock()
	}
}

func runReader(t *testing.T, scoped bool) (reads int, bad []string) {
	d := newRepo(t, 5000)
	write(t, d, "reader-untracked.txt", "r")
	stop := make(chan struct{})
	var mu sync.Mutex
	done := make(chan struct{})
	go func() { readerLoop(d, stop, &bad, &reads, &mu); close(done) }()
	for i := 0; i < 200; i++ {
		f := fmt.Sprintf("w%03d.txt", i)
		write(t, d, f, "w")
		if scoped {
			err := WithIndexLock(d, Options{}, func(sc *Scope) error { _, e := sc.WT.Add(f); return e })
			if err != nil {
				if _, ok := err.(*ErrLocked); ok { // CLI reader held lock; retry
					i--
					continue
				}
				t.Fatal(err)
			}
		} else {
			r, _ := git.PlainOpen(d)
			w, _ := r.Worktree()
			if _, err := w.Add(f); err != nil {
				mu.Lock()
				bad = append(bad, "stock Add err: "+err.Error())
				mu.Unlock()
			}
		}
	}
	close(stop)
	<-done
	if scoped {
		noLocks(t, d)
		if n := len(lsFiles(t, d)); n < 5200 {
			t.Fatalf("entries %d < 5200", n)
		}
	}
	return
}

func TestT3a_PartialRead_Scoped(t *testing.T) {
	reads, bad := runReader(t, true)
	t.Logf("T3 scoped: readerInvocations=%d anomalies=%d %v", reads, len(bad), first(bad))
	if len(bad) > 0 {
		t.Fatalf("anomalies: %v", first(bad))
	}
}

func TestT3b_PartialRead_StockInPlace(t *testing.T) {
	reads, bad := runReader(t, false)
	t.Logf("T3 stock (expected reproduction): readerInvocations=%d anomalies=%d %v", reads, len(bad), first(bad))
}

func first(b []string) []string {
	if len(b) > 3 {
		return b[:3]
	}
	return b
}

var _ = os.Stdout
var _ = exec.Command
