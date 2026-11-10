package scenario

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/tools/promolib/termscan"
)

// fakeDefaultPathPrefixes lists the obviously fake paths the legacy default
// scenario (tests/demo mockSessions) uses; it is not rewritten, so it is
// allow-listed here.
func fakeDefaultPathPrefixes() []string { return []string{"/Users/dev/"} }

func leakHits(text string) []string {
	for _, p := range fakeDefaultPathPrefixes() {
		text = strings.ReplaceAll(text, p, "")
	}
	var hits []string
	for _, p := range termscan.LeakPatterns() {
		if m := p.Find(text); m != "" {
			hits = append(hits, m)
		}
	}
	return hits
}

func TestSeedDataHasNoLeaks(t *testing.T) {
	t.Parallel()
	for _, n := range Names() {
		if n == DefaultName {
			continue
		}
		b, err := BuildBundle(n, refA)
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		if hits := leakHits(string(data)); len(hits) > 0 {
			t.Errorf("scenario %s leaks %v", n, hits)
		}
	}
}

func TestTranscriptsHaveNoLeaks(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir("testdata")
	if err != nil || len(entries) == 0 {
		t.Fatalf("no transcripts: %v", err)
	}
	for _, e := range entries {
		data, _ := os.ReadFile("testdata/" + e.Name())
		if hits := leakHits(string(data)); len(hits) > 0 {
			t.Errorf("%s leaks %v", e.Name(), hits)
		}
	}
}

func TestLeakPatternsCatchKnownBadStrings(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"/home/alice/x", "/tmp/tmp.abc", "ghp_abcdef123", "box.internal", "a@b.com", "/Users/alice/x"} {
		if len(leakHits(bad)) == 0 {
			t.Errorf("pattern set misses %q", bad)
		}
	}
	if len(leakHits("/Users/dev/services/x")) != 0 {
		t.Error("allow-listed fake default path should pass")
	}
}

func TestDefaultScenarioPathsAreAllowListed(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("../helpers.go")
	if err != nil {
		t.Fatal(err)
	}
	paths := regexp.MustCompile(`"(/Users/[^"]+)"`).FindAllStringSubmatch(string(src), -1)
	if len(paths) == 0 {
		t.Fatal("no default scenario paths found; the extraction regexp is stale")
	}
	for _, m := range paths {
		if hits := leakHits(m[1]); len(hits) > 0 {
			t.Errorf("default path %s is not allow-listed: %v", m[1], hits)
		}
	}
}
