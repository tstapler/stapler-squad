package services

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/pkg/classifier"
)

func TestHookContextCache_Get_should_CoalesceRepositoryLookup_When_CwdMissesAreConcurrent(t *testing.T) {
	var calls atomic.Int64
	started := make(chan struct{})
	release := make(chan struct{})
	cache := NewHookContextCache(func(cwd string) classifier.ClassificationContext {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return classifier.ClassificationContext{Cwd: cwd, IsGitRepo: true, RepoRoot: "/repo"}
	})

	const requestCount = 100
	results := make(chan classifier.ClassificationContext, requestCount)
	var wait sync.WaitGroup
	wait.Add(requestCount)
	for range requestCount {
		go func() {
			defer wait.Done()
			results <- cache.Get("/repo/subdir")
		}()
	}
	<-started
	close(release)
	wait.Wait()
	close(results)

	if calls.Load() != 1 {
		t.Fatalf("lookup calls = %d, want 1", calls.Load())
	}
	for result := range results {
		if !result.IsGitRepo || result.RepoRoot != "/repo" {
			t.Fatalf("unexpected context: %+v", result)
		}
	}
	_ = cache.Get("/repo/subdir")
	if calls.Load() != 1 {
		t.Fatalf("warm lookup calls = %d, want 1", calls.Load())
	}
}

func TestHookContextCache_Get_should_ReturnStaleContextWithinBudget_When_RefreshIsSlow(t *testing.T) {
	now := time.Unix(100, 0)
	var calls atomic.Int64
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	cache := NewHookContextCache(func(cwd string) classifier.ClassificationContext {
		call := calls.Add(1)
		if call > 1 {
			close(refreshStarted)
			<-releaseRefresh
		}
		return classifier.ClassificationContext{Cwd: cwd, RepoRoot: "/repo"}
	}, WithHookContextClock(func() time.Time { return now }), WithHookContextAges(time.Second, time.Minute))

	initial := cache.Get("/repo")
	now = now.Add(3 * time.Second)
	started := time.Now()
	stale := cache.Get("/repo")
	if elapsed := time.Since(started); elapsed > 20*time.Millisecond {
		t.Fatalf("stale lookup took %s, want <20ms", elapsed)
	}
	if stale.RepoRoot != initial.RepoRoot {
		t.Fatalf("stale context = %+v, initial = %+v", stale, initial)
	}
	select {
	case <-refreshStarted:
	case <-time.After(time.Second):
		t.Fatal("asynchronous refresh did not start")
	}
	close(releaseRefresh)
}
