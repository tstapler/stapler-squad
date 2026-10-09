// S6 spike: go-git reads vs concurrent repack/gc/pack-refs.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-billy/v5/osfs"
)

func run(dir string, args ...string) error {
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v: %v: %s", args, err, out)
	}
	return nil
}

func mkRepo(dir string) {
	os.RemoveAll(dir)
	os.MkdirAll(dir, 0o755)
	must(run(dir, "init", "-q", "-b", "main"))
	for i := 0; i < 50; i++ {
		os.WriteFile(dir+"/f.txt", []byte(fmt.Sprintf("v%d\n", i)), 0o644)
		must(run(dir, "add", "f.txt"))
		must(run(dir, "commit", "-q", "-m", fmt.Sprintf("c%d", i)))
	}
	must(run(dir, "repack", "-ad"))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// openRepo mimics a fresh per-call OpenRepo (no cached handle).
func openRepo(dir string) (*git.Repository, error) { return git.PlainOpen(dir) }

func read(r *git.Repository, h plumbing.Hash) error {
	c, err := r.CommitObject(h)
	if err != nil {
		return err
	}
	_, err = c.Tree()
	return err
}

type stats struct {
	reads, notFound, otherErr, retried, retryFail int64
	maxAttempts                                    int64
	mu                                             sync.Mutex
	lat                                            []time.Duration // latency of reads that needed a retry
	errKinds                                       map[string]int
}

func (s *stats) kind(e error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.errKinds == nil {
		s.errKinds = map[string]int{}
	}
	k := e.Error()
	if len(k) > 90 {
		k = k[:90]
	}
	// normalise paths/hashes
	s.errKinds[k]++
}

func main() {
	mode := flag.String("mode", "stock", "stock|fresh|fresh-retry|reindex-retry|reindex-retry-locked")
	maint := flag.String("maint", "repack", "repack|gc|packrefs")
	dur := flag.Duration("dur", 60*time.Second, "duration")
	dir := flag.String("dir", "/tmp/s6repo", "repo dir")
	readers := flag.Int("readers", 8, "readers")
	flag.Parse()
	mkRepo(*dir)

	hb, _ := exec.Command("git", "-C", *dir, "rev-parse", "HEAD").Output()
	head := plumbing.NewHash(strings.TrimSpace(string(hb)))

	var stop atomic.Bool
	var mcount atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			var err error
			switch *maint {
			case "repack":
				err = run(*dir, "repack", "-ad")
			case "gc":
				err = run(*dir, "gc", "--quiet", "--prune=now", "--aggressive")
			case "packrefs":
				// pack-refs alone does not move objects; combine with a loose-ref churn so refs are rewritten.
				_ = run(*dir, "update-ref", "refs/heads/churn", "HEAD")
				err = run(*dir, "pack-refs", "--all", "--prune")
			}
			if err != nil {
				fmt.Println("maint err:", err)
			}
			mcount.Add(1)
		}
	}()

	st := &stats{}
	shared, err := openRepo(*dir)
	must(err)
	var sharedMu sync.Mutex // used only by reindex-retry-locked

	for i := 0; i < *readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				atomic.AddInt64(&st.reads, 1)
				start := time.Now()
				var err error
				attempts := int64(1)
				switch *mode {
				case "stock":
					err = read(shared, head)
				case "fresh":
					var r *git.Repository
					if r, err = openRepo(*dir); err == nil {
						err = read(r, head)
					}
				case "fresh-retry":
					var r *git.Repository
					if r, err = openRepo(*dir); err == nil {
						err = read(r, head)
					}
					for err != nil && attempts < 5 {
						attempts++
						if r, err = openRepo(*dir); err == nil {
							err = read(r, head)
						}
					}
				case "reindex-retry":
					err = read(shared, head)
					for err != nil && attempts < 5 {
						attempts++
						if fs, ok := shared.Storer.(*filesystem.Storage); ok {
							fs.Reindex()
						}
						err = read(shared, head)
					}
				case "reindex-retry-locked":
					sharedMu.Lock()
					err = read(shared, head)
					for err != nil && attempts < 5 {
						attempts++
						if fs, ok := shared.Storer.(*filesystem.Storage); ok {
							fs.Reindex()
						}
						err = read(shared, head)
					}
					sharedMu.Unlock()
				}
				if attempts > 1 {
					atomic.AddInt64(&st.retried, 1)
					st.mu.Lock()
					st.lat = append(st.lat, time.Since(start))
					st.mu.Unlock()
					for {
						m := atomic.LoadInt64(&st.maxAttempts)
						if attempts <= m || atomic.CompareAndSwapInt64(&st.maxAttempts, m, attempts) {
							break
						}
					}
				}
				if err != nil {
					if errors.Is(err, plumbing.ErrObjectNotFound) {
						atomic.AddInt64(&st.notFound, 1)
					} else {
						atomic.AddInt64(&st.otherErr, 1)
					}
					if attempts > 1 {
						atomic.AddInt64(&st.retryFail, 1)
					}
					st.kind(err)
				}
			}
		}()
	}
	time.Sleep(*dur)
	stop.Store(true)
	wg.Wait()
	sort.Slice(st.lat, func(i, j int) bool { return st.lat[i] < st.lat[j] })
	p := func(q float64) time.Duration {
		if len(st.lat) == 0 {
			return 0
		}
		return st.lat[int(float64(len(st.lat)-1)*q)]
	}
	fmt.Printf("mode=%s maint=%s dur=%s maint_runs=%d reads=%d ErrObjectNotFound=%d otherErr=%d retried_reads=%d final_failures_after_retry=%d max_attempts=%d retry_latency_p50=%s p99=%s max=%s\n",
		*mode, *maint, *dur, mcount.Load(), st.reads, st.notFound, st.otherErr, st.retried, st.retryFail, st.maxAttempts, p(.5), p(.99), p(1))
	for k, v := range st.errKinds {
		fmt.Printf("  err x%d: %s\n", v, k)
	}
	_ = cache.NewObjectLRUDefault
	_ = osfs.New
}
