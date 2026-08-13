package services

import (
	"testing"
	"time"

	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/tstapler/stapler-squad/testutil"
)

func makePatterns(lines ...string) []gitignore.Pattern {
	patterns := make([]gitignore.Pattern, 0, len(lines))
	for _, l := range lines {
		patterns = append(patterns, gitignore.ParsePattern(l, nil))
	}
	return patterns
}

func TestGitignoreCache_Hit(t *testing.T) {
	c := NewGitignoreCache(10, 5*time.Second)
	want := makePatterns("*.log", "node_modules/")

	c.Put("root:/some/dir", want, time.Now())

	got, ok := c.Get("root:/some/dir")
	if !ok {
		t.Fatal("expected cache hit, got miss")
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d patterns, got %d", len(want), len(got))
	}
}

func TestGitignoreCache_MissOnTTLExpiry(t *testing.T) {
	ttl := 10 * time.Millisecond
	c := NewGitignoreCache(10, ttl)
	c.Put("root:/some/dir", makePatterns("*.log"), time.Now())

	// Poll until the TTL has expired and Get returns a miss.
	if err := testutil.WaitForCondition(func() bool {
		_, ok := c.Get("root:/some/dir")
		return !ok
	}, testutil.FastWaitConfig()); err != nil {
		t.Fatal("expected cache miss after TTL expiry, got hit")
	}
}

func TestGitignoreCache_Eviction(t *testing.T) {
	maxSize := 3
	c := NewGitignoreCache(maxSize, 5*time.Second)

	// Fill the cache to capacity with distinct keys.
	for i := 0; i < maxSize; i++ {
		key := string(rune('a'+i)) + ":/dir"
		c.Put(key, makePatterns("*.tmp"), time.Now())
		// Small delay so cachedAt timestamps are strictly ordered.
		<-time.After(1 * time.Millisecond)
	}

	// Verify all entries are present.
	for i := 0; i < maxSize; i++ {
		key := string(rune('a'+i)) + ":/dir"
		if _, ok := c.Get(key); !ok {
			t.Fatalf("expected cache hit for key %q before eviction", key)
		}
	}

	// Adding one more entry should evict the oldest (key "a:/dir").
	c.Put("new:/dir", makePatterns("*.bak"), time.Now())

	if _, ok := c.Get("a:/dir"); ok {
		t.Fatal("expected oldest entry 'a:/dir' to be evicted")
	}
	if _, ok := c.Get("new:/dir"); !ok {
		t.Fatal("expected new entry 'new:/dir' to be present after eviction")
	}
}
