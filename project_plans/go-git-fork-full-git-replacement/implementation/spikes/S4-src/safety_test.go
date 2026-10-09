package s4lock

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"fmt"
)

func indexBytes(t *testing.T, d string) []byte {
	b, err := os.ReadFile(filepath.Join(d, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestT4_ErrorAndPanicNoLeak(t *testing.T) {
	d := newRepo(t, 50)
	before := indexBytes(t, d)
	write(t, d, "n.txt", "n")
	// error after SetIndex was called (pending content in lock) and before rename
	err := WithIndexLock(d, Options{}, func(sc *Scope) error {
		if _, e := sc.WT.Add("n.txt"); e != nil {
			return e
		}
		return errors.New("injected")
	})
	if err == nil || err.Error() != "injected" {
		t.Fatalf("got %v", err)
	}
	noLocks(t, d)
	if string(indexBytes(t, d)) != string(before) {
		t.Fatal("index changed after aborted scope")
	}
	// panic
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic to propagate")
			}
		}()
		WithIndexLock(d, Options{}, func(sc *Scope) error { sc.WT.Add("n.txt"); panic("boom") })
	}()
	noLocks(t, d)
	if string(indexBytes(t, d)) != string(before) {
		t.Fatal("index changed after panic")
	}
	// read-only scope takes lock but never changes index and leaves nothing
	if err := WithIndexLock(d, Options{}, func(sc *Scope) error { _, e := sc.WT.Status(); return e }); err != nil {
		t.Fatal(err)
	}
	noLocks(t, d)
	if string(indexBytes(t, d)) != string(before) {
		t.Fatal("index changed by read-only scope")
	}
	// BeforeCommitPhase injection error
	err = WithIndexLock(d, Options{BeforeCommitPhase: func() error { return errors.New("inj2") }},
		func(sc *Scope) error { _, e := sc.WT.Add("n.txt"); return e })
	if err == nil {
		t.Fatal("want err")
	}
	noLocks(t, d)
	if string(indexBytes(t, d)) != string(before) {
		t.Fatal("index changed")
	}
}

func TestT5_ForeignLockNeverDeleted(t *testing.T) {
	d := newRepo(t, 10)
	lock := filepath.Join(d, ".git", "index.lock")
	for _, age := range []time.Duration{0, 10 * time.Minute} {
		os.WriteFile(lock, []byte("foreign"), 0o644)
		old := time.Now().Add(-age)
		os.Chtimes(lock, old, old)
		err := WithIndexLock(d, Options{}, func(sc *Scope) error { t.Fatal("fn must not run"); return nil })
		var el *ErrLocked
		if !errors.As(err, &el) {
			t.Fatalf("want ErrLocked got %v", err)
		}
		b, rerr := os.ReadFile(lock)
		if rerr != nil || string(b) != "foreign" {
			t.Fatalf("foreign lock deleted/modified (age %v)", age)
		}
		t.Logf("T5 age=%v: %v (lock untouched)", age, err)
		os.Remove(lock)
	}
}

// T6: deadlock check: ssq-level mutex (repoWorktreeLock stand-in) then index.lock, 24 goroutines.
func TestT6_NoDeadlock(t *testing.T) {
	d := newRepo(t, 100)
	var mu sync.Mutex // repoWorktreeLock stand-in: order is mu -> index.lock
	var wg sync.WaitGroup
	done := make(chan struct{})
	go func() {
		for g := 0; g < 24; g++ {
			f := fmt.Sprintf("p%02d.txt", g)
			write(t, d, f, "p")
			wg.Add(1)
			go func() {
				defer wg.Done()
				mu.Lock()
				defer mu.Unlock()
				for {
					err := WithIndexLock(d, Options{}, func(sc *Scope) error { _, e := sc.WT.Add(f); return e })
					if _, ok := err.(*ErrLocked); ok {
						time.Sleep(time.Millisecond)
						continue
					}
					if err != nil {
						t.Error(err)
					}
					return
				}
			}()
		}
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("deadlock/timeout")
	}
	noLocks(t, d)
	// also without the outer mutex: 24 goroutines contending index.lock directly
	var wg2 sync.WaitGroup
	for g := 0; g < 24; g++ {
		f := fmt.Sprintf("q%02d.txt", g)
		write(t, d, f, "q")
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			for {
				err := WithIndexLock(d, Options{}, func(sc *Scope) error { _, e := sc.WT.Add(f); return e })
				if _, ok := err.(*ErrLocked); ok {
					time.Sleep(time.Millisecond)
					continue
				}
				if err != nil {
					t.Error(err)
				}
				return
			}
		}()
	}
	wg2.Wait()
	noLocks(t, d)
	if n := len(lsFiles(t, d)); n != 100+48 {
		t.Fatalf("entries %d != 148", n)
	}
}
