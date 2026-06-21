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
