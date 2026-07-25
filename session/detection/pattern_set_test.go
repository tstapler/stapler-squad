package detection

import (
	"sync"
	"testing"
)

func TestNewPatternSet_should_returnError_When_invalidRegexProvided(t *testing.T) {
	p := StatusPatterns{
		Ready: []StatusPattern{{Name: "bad", Pattern: "(?P<invalid", Description: "invalid"}},
	}
	_, err := NewPatternSet(p)
	if err == nil {
		t.Error("expected error for invalid regex, got nil")
	}
}

func TestPatternSet_MatchLines_should_returnError_When_errorStringPresent(t *testing.T) {
	ps, err := NewPatternSet(getDefaultPatterns())
	if err != nil {
		t.Fatal(err)
	}
	status, name, _ := ps.MatchLines("Error: something went wrong", nil)
	if status != StatusError {
		t.Errorf("got status %v, want StatusError", status)
	}
	if name == "" {
		t.Error("expected non-empty pattern name")
	}
}

func TestPatternSet_MatchLines_should_returnUnknown_When_noMatchAndCatchAll(t *testing.T) {
	// After Epic 2, the .* catch-all in the Ready category returns StatusUnknown
	// so that unrecognized output renders no badge (not "Ready").
	ps, err := NewPatternSet(getDefaultPatterns())
	if err != nil {
		t.Fatal(err)
	}
	status, _, _ := ps.MatchLines("some generic output with no pattern match", nil)
	if status != StatusUnknown {
		t.Errorf("got status %v, want StatusUnknown (catch-all renders no badge)", status)
	}
}

func TestPatternSet_MatchLines_should_returnReady_When_explicitReadyPatternMatches(t *testing.T) {
	// Regression for adversarial-review.md Issue 4: the Ready bucket contains both an
	// explicit named pattern (gemini_ready) and the universal `.*` catch-all
	// (claude_prompt) at an earlier slice index. Before the fix, the catch-all matched
	// every string first and the whole bucket unconditionally returned StatusUnknown,
	// making gemini_ready unreachable dead code. Explicit ready patterns must now be
	// checked before the catch-all and return StatusReady, not StatusUnknown.
	ps, err := NewPatternSet(getDefaultPatterns())
	if err != nil {
		t.Fatal(err)
	}
	status, name, _ := ps.MatchLines("◇ Ready", nil)
	if status != StatusReady {
		t.Errorf("got status %v, want StatusReady for explicit gemini_ready match", status)
	}
	if name != "gemini_ready" {
		t.Errorf("got pattern name %q, want %q", name, "gemini_ready")
	}
}

func TestPatternSet_MatchLines_should_returnUnknown_When_catchAllPatternMatchesAfterExplicitReadyChecked(t *testing.T) {
	// Complements the above: text that matches neither an explicit Ready pattern nor
	// any earlier-priority category must still fall through to the `.*` catch-all and
	// return StatusUnknown (not StatusReady), confirming the catch-all remains reachable
	// and last in the split Ready checks.
	ps, err := NewPatternSet(getDefaultPatterns())
	if err != nil {
		t.Fatal(err)
	}
	status, name, _ := ps.MatchLines("just some plain prompt text", nil)
	if status != StatusUnknown {
		t.Errorf("got status %v, want StatusUnknown for catch-all match", status)
	}
	if name != "claude_prompt" {
		t.Errorf("got pattern name %q, want %q", name, "claude_prompt")
	}
}

func TestPatternSet_MatchLines_should_beRaceFree_When_calledConcurrently(t *testing.T) {
	ps, err := NewPatternSet(getDefaultPatterns())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				ps.MatchLines("Thinking...", nil)
			}
		}()
	}
	wg.Wait()
}
