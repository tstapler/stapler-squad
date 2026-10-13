//go:build gitoracle

package gitconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/testutil/gitoracle"
)

// patternsOver returns every pattern of 1..maxTokens tokens from the alphabet.
func patternsOver(tokens []string, maxTokens int) []string {
	out := []string(nil)
	level := []string{""}
	for range maxTokens {
		var next []string
		for _, p := range level {
			for _, tok := range tokens {
				next = append(next, p+tok)
			}
		}
		out = append(out, next...)
		level = next
	}
	return out
}

func textsOver(alphabet string, maxLen int) []string {
	out := []string(nil)
	level := []string{""}
	for range maxLen {
		var next []string
		for _, p := range level {
			for _, c := range alphabet {
				next = append(next, p+string(c))
			}
		}
		out = append(out, next...)
		level = next
	}
	return out
}

// TestMatchAgreesWithGitHasconfig drives git's real wildmatch: for each remote URL text, a
// config holds one `includeIf "hasconfig:remote.*.url:<pattern>"` per pattern, each including a
// file that sets a key unique to that pattern, and the keys that appear are the patterns git
// matched. Match must give the same set.
func TestMatchAgreesWithGitHasconfig(t *testing.T) {
	gitoracle.RequireGit(t)
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	// The default set takes seconds; SSQ_WILDMATCH_EXHAUSTIVE=1 runs the larger one (about two
	// minutes), which agreed with git when this port was written.
	maxTokens, maxText := 3, 5
	if os.Getenv("SSQ_WILDMATCH_EXHAUSTIVE") == "1" {
		maxTokens, maxText = 4, 6
	}
	patterns := patternsOver([]string{"a", "/", "*", "**", "?"}, maxTokens)
	var cfg strings.Builder
	for i, p := range patterns {
		writeTestFile(t, filepath.Join(home, fmt.Sprintf("t%d.cfg", i)), fmt.Sprintf("[m]\n\tp%d = 1\n", i))
		fmt.Fprintf(&cfg, "[includeIf \"hasconfig:remote.*.url:%s\"]\n\tpath = %s\n", p, filepath.Join(home, fmt.Sprintf("t%d.cfg", i)))
	}
	writeTestFile(t, filepath.Join(home, ".gitconfig"), cfg.String())

	for _, text := range textsOver("a/", maxText) {
		env := append(cleanEnv(home), "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=remote.x.url", "GIT_CONFIG_VALUE_0="+text)
		out, stderr, err := gitRun(t, home, env, "config", "--list", "-z")
		require.NoError(t, err, "text %q: %s", text, stderr)
		hit := map[string]bool{}
		for _, row := range parseZList(out) {
			if k, _, ok := strings.Cut(row, "="); ok && strings.HasPrefix(k, "m.p") {
				hit[k[len("m.p"):]] = true
			}
		}
		var mismatches []string
		for i, p := range patterns {
			got, ok := Match(p, text, false)
			require.True(t, ok)
			if got != hit[fmt.Sprint(i)] {
				mismatches = append(mismatches, fmt.Sprintf("pattern %q: Match=%v git=%v", p, got, hit[fmt.Sprint(i)]))
			}
		}
		assert.Empty(t, mismatches, "text %q", text)
	}
}

// TestMatchCaseFoldAgreesWithGitdirI checks gitdir/i: folding on a real path.
func TestMatchCaseFoldAgreesWithGitdirI(t *testing.T) {
	f := newFixture(t)
	f.write("cond.cfg", "[zz]\n\thit = yes\n")
	for _, c := range []struct {
		cond string
		want bool
	}{
		{"gitdir/i:" + strings.ToUpper(f.root) + "/", true},
		{"gitdir:" + strings.ToUpper(f.root) + "/", false},
		{"gitdir/i:" + strings.ToUpper(f.home) + "/**/.git", true},
	} {
		f.write(".gitconfig", "[includeIf \""+c.cond+"\"]\n\tpath = "+filepath.Join(f.home, "cond.cfg")+"\n")
		out, _, err := gitRun(t, f.root, cleanEnv(f.home), "config", "--get", "zz.hit")
		hit := err == nil && strings.TrimSpace(out) == "yes"
		assert.Equal(t, c.want, hit, c.cond)
	}
}
