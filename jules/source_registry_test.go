package jules

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSourceLister is a sourceLister test double that counts ListSources
// calls and returns a fixed source list.
type fakeSourceLister struct {
	sources        []JulesSource
	listSources    int
	listSourcesErr error
}

func (f *fakeSourceLister) ListSources(ctx context.Context) ([]JulesSource, error) {
	f.listSources++
	if f.listSourcesErr != nil {
		return nil, f.listSourcesErr
	}
	return f.sources, nil
}

// blockingSourceLister is a sourceLister test double whose ListSources call
// blocks on a channel until released, letting a test hold a call in flight
// while other goroutines race to call it concurrently.
type blockingSourceLister struct {
	sources []JulesSource
	calls   int32
	block   chan struct{}
	entered chan struct{}
}

func (f *blockingSourceLister) ListSources(ctx context.Context) ([]JulesSource, error) {
	atomic.AddInt32(&f.calls, 1)
	select {
	case f.entered <- struct{}{}:
	default:
	}
	<-f.block
	return f.sources, nil
}

func TestJulesSourceRegistry_Resolve_should_ServeFromCacheWithoutSecondCall_When_CalledWithinTTL(t *testing.T) {
	client := &fakeSourceLister{
		sources: []JulesSource{
			{Name: "sources/github-tstapler-stapler-squad", ID: "github-tstapler-stapler-squad"},
		},
	}
	registry := NewJulesSourceRegistry(client)

	first, err := registry.Resolve(context.Background(), "tstapler", "stapler-squad")
	if err != nil {
		t.Fatalf("first Resolve: unexpected error: %v", err)
	}

	second, err := registry.Resolve(context.Background(), "tstapler", "stapler-squad")
	if err != nil {
		t.Fatalf("second Resolve: unexpected error: %v", err)
	}

	if first != second {
		t.Errorf("expected the same JulesSourceName from both calls, got %q then %q", first, second)
	}
	if want := JulesSourceName("sources/github-tstapler-stapler-squad"); first != want {
		t.Errorf("Resolve() = %q, want %q", first, want)
	}
	if client.listSources != 1 {
		t.Errorf("client.listSources = %d, want exactly 1 ListSources call", client.listSources)
	}
}

func TestJulesSourceRegistry_Resolve_should_ReturnErrJulesSourceNotRegistered_When_RepoAbsentFromSources(t *testing.T) {
	client := &fakeSourceLister{
		sources: []JulesSource{
			{Name: "sources/github-tstapler-dotfiles", ID: "github-tstapler-dotfiles"},
		},
	}
	registry := NewJulesSourceRegistry(client)

	_, err := registry.Resolve(context.Background(), "tstapler", "stapler-squad")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, ErrJulesSourceNotRegistered) {
		t.Errorf("errors.Is(err, ErrJulesSourceNotRegistered) = false, err = %v", err)
	}
	if !strings.Contains(err.Error(), "tstapler/stapler-squad") {
		t.Errorf("error %q does not name tstapler/stapler-squad", err.Error())
	}
	if !strings.Contains(err.Error(), "jules.google.com") {
		t.Errorf("error %q does not instruct the user to connect the repo at jules.google.com", err.Error())
	}
}

func TestJulesSourceRegistry_Resolve_should_RefetchSources_When_TTLExpired(t *testing.T) {
	client := &fakeSourceLister{
		sources: []JulesSource{
			{Name: "sources/github-tstapler-stapler-squad", ID: "github-tstapler-stapler-squad"},
		},
	}
	registry := NewJulesSourceRegistry(client)
	registry.TTL = 10 * time.Minute

	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	registry.now = func() time.Time { return now }

	_, err := registry.Resolve(context.Background(), "tstapler", "stapler-squad")
	if err != nil {
		t.Fatalf("first Resolve: unexpected error: %v", err)
	}
	if client.listSources != 1 {
		t.Fatalf("client.listSources = %d after first Resolve, want 1", client.listSources)
	}

	now = now.Add(11 * time.Minute)

	_, err = registry.Resolve(context.Background(), "tstapler", "stapler-squad")
	if err != nil {
		t.Fatalf("second Resolve: unexpected error: %v", err)
	}
	if client.listSources != 2 {
		t.Errorf("client.listSources = %d after TTL expiry, want 2 (a second ListSources call)", client.listSources)
	}
}

// TestJulesSourceRegistry_Resolve_should_CoalesceConcurrentMisses_When_ManyGoroutinesRaceOnColdCache
// exercises refetchGroup: N goroutines call Resolve concurrently against a
// registry with no cache entries yet, so every goroutine independently
// misses the cache and calls refresh(). Before the singleflight fix, each
// waiter on the hand-rolled refetchMu unconditionally re-called ListSources
// after acquiring the lock instead of re-checking whether the goroutine
// ahead of it had already populated the entry -- this asserts exactly one
// ListSources call serves all of them.
func TestJulesSourceRegistry_Resolve_should_CoalesceConcurrentMisses_When_ManyGoroutinesRaceOnColdCache(t *testing.T) {
	client := &blockingSourceLister{
		sources: []JulesSource{
			{Name: "sources/github-tstapler-stapler-squad", ID: "github-tstapler-stapler-squad"},
		},
		block:   make(chan struct{}),
		entered: make(chan struct{}, 1),
	}
	registry := NewJulesSourceRegistry(client)

	const n = 50
	var startWG, doneWG sync.WaitGroup
	startWG.Add(n)
	doneWG.Add(n)
	start := make(chan struct{})
	results := make([]JulesSourceName, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer doneWG.Done()
			startWG.Done()
			<-start
			results[i], errs[i] = registry.Resolve(context.Background(), "tstapler", "stapler-squad")
		}(i)
	}

	startWG.Wait()
	close(start)
	select {
	case <-client.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the single ListSources call to start")
	}
	time.Sleep(50 * time.Millisecond)
	close(client.block)

	doneWG.Wait()

	if got := atomic.LoadInt32(&client.calls); got != 1 {
		t.Fatalf("client.ListSources invoked %d times across %d goroutines racing a cold cache, want exactly 1", got, n)
	}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: Resolve: %v", i, err)
		}
		if want := JulesSourceName("sources/github-tstapler-stapler-squad"); results[i] != want {
			t.Fatalf("goroutine %d: Resolve = %q, want %q", i, results[i], want)
		}
	}
}

func TestParseGitHubSourceName_should_SplitOwnerAndRepo_When_TableDriven(t *testing.T) {
	tests := []struct {
		name      string
		source    JulesSourceName
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		{
			name:      "simple repo name",
			source:    "sources/github-tstapler-dotfiles",
			wantOwner: "tstapler",
			wantRepo:  "dotfiles",
			wantOK:    true,
		},
		{
			name:      "hyphenated repo name",
			source:    "sources/github-tstapler-stapler-squad",
			wantOwner: "tstapler",
			wantRepo:  "stapler-squad",
			wantOK:    true,
		},
		{
			name:   "missing sources/github- prefix",
			source: "sources/gitlab-tstapler-dotfiles",
			wantOK: false,
		},
		{
			name:   "no repo segment",
			source: "sources/github-tstapler",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, ok := parseGitHubSourceName(tt.source)
			if ok != tt.wantOK {
				t.Fatalf("parseGitHubSourceName(%q) ok = %v, want %v", tt.source, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if owner != tt.wantOwner || repo != tt.wantRepo {
				t.Errorf("parseGitHubSourceName(%q) = (%q, %q), want (%q, %q)", tt.source, owner, repo, tt.wantOwner, tt.wantRepo)
			}
		})
	}
}
