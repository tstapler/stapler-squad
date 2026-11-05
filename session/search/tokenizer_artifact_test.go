package search

import (
	"slices"
	"testing"
)

// The base tokenizer splits "acme/app#42" into acme, app and 42, so "#42" and
// "#43" share two of three tokens and "feat-a" equals "feat-b" once the
// one-letter suffix is dropped. TokenizeArtifact keeps the identifier whole too.
func TestTokenizeArtifact_ShouldKeepWholeIdentifiers(t *testing.T) {
	t.Parallel()
	tok := NewTokenizer()
	tests := []struct {
		name, in, want string
	}{
		{"pr ref", "see Acme/App#42 now", "acme/app#42"},
		{"short sha", "1a2b3c4", "1a2b3c4"},
		{"branch", "pushed feat-b to origin", "feat-b"},
		{"plan path", "wrote docs/plans/foo.md.", "docs/plans/foo.md"},
		{"url in parens", "(https://github.com/acme/app/pull/12)", "https://github.com/acme/app/pull/12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tok.TokenizeArtifact(tt.in); !slices.Contains(got, tt.want) {
				t.Fatalf("TokenizeArtifact(%q) = %v, want it to contain %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestTokenizeArtifact_ShouldKeepTheBaseTokens(t *testing.T) {
	t.Parallel()
	tok := NewTokenizer()
	got := tok.TokenizeArtifact("acme/app#42")
	for _, want := range tok.Tokenize("acme/app#42") {
		if !slices.Contains(got, want) {
			t.Fatalf("TokenizeArtifact lost base token %q: %v", want, got)
		}
	}
}

func TestTokenizeArtifact_ShouldNotDuplicateOrEmitPlainWords(t *testing.T) {
	t.Parallel()
	got := NewTokenizer().TokenizeArtifact("deploy deploy acme/app#1 acme/app#1")
	seen := map[string]int{}
	for _, g := range got {
		seen[g]++
		if seen[g] > 1 {
			t.Fatalf("duplicate token %q in %v", g, got)
		}
	}
	if want := NewTokenizer().Tokenize("deploy")[0]; !slices.Contains(got, want) {
		t.Fatalf("stemmed plain word %q missing from %v", want, got)
	}
}
