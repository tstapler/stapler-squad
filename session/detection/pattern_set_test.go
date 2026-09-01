package detection

import (
	"sync"
	"testing"
)

func TestNewPatternSet_should_returnError_When_invalidRegexProvided(t *testing.T) {
	t.Parallel()
	p := StatusPatterns{
		Ready: []StatusPattern{{Name: "bad", Pattern: "(?P<invalid", Description: "invalid"}},
	}
	_, err := NewPatternSet(p)
	if err == nil {
		t.Error("expected error for invalid regex, got nil")
	}
}

func TestPatternSet_MatchLines_should_returnError_When_errorStringPresent(t *testing.T) {
	t.Parallel()
	ps, err := NewPatternSet(getDefaultPatterns())
	if err != nil {
		t.Fatal(err)
	}
	status, name, _, _ := ps.MatchLines("Error: something went wrong", nil)
	if status != StatusError {
		t.Errorf("got status %v, want StatusError", status)
	}
	if name == "" {
		t.Error("expected non-empty pattern name")
	}
}

func TestPatternSet_MatchLines_should_returnUnknown_When_noMatchAndCatchAll(t *testing.T) {
	t.Parallel()
	// After Epic 2, the .* catch-all in the Ready category returns StatusUnknown
	// so that unrecognized output renders no badge (not "Ready").
	ps, err := NewPatternSet(getDefaultPatterns())
	if err != nil {
		t.Fatal(err)
	}
	status, _, _, _ := ps.MatchLines("some generic output with no pattern match", nil)
	if status != StatusUnknown {
		t.Errorf("got status %v, want StatusUnknown (catch-all renders no badge)", status)
	}
}

func TestPatternSet_MatchLines_should_returnCount_When_waitingForBackgroundAgentMatches(t *testing.T) {
	t.Parallel()
	ps, err := NewPatternSet(getDefaultPatterns())
	if err != nil {
		t.Fatal(err)
	}
	status, _, _, count := ps.MatchLines("✻ Waiting for 2 background agents to finish", nil)
	if status != StatusWaitingForAgent {
		t.Errorf("got status %v, want StatusWaitingForAgent", status)
	}
	if count != 2 {
		t.Errorf("got count %d, want 2", count)
	}
}

func TestPatternSet_MatchLines_should_returnCount_When_shellsStillRunningMatches(t *testing.T) {
	t.Parallel()
	ps, err := NewPatternSet(getDefaultPatterns())
	if err != nil {
		t.Fatal(err)
	}
	status, _, _, count := ps.MatchLines("1 shell still running", nil)
	if status != StatusWaitingForAgent {
		t.Errorf("got status %v, want StatusWaitingForAgent", status)
	}
	if count != 1 {
		t.Errorf("got count %d, want 1", count)
	}
}

func TestPatternSet_MatchLines_should_returnZero_When_noWaitingForAgentPatternMatches(t *testing.T) {
	t.Parallel()
	ps, err := NewPatternSet(getDefaultPatterns())
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, count := ps.MatchLines("Thinking...", nil)
	if count != 0 {
		t.Errorf("got count %d, want 0", count)
	}
}

func TestPatternSet_MatchLines_should_beRaceFree_When_calledConcurrently(t *testing.T) {
	t.Parallel()
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
