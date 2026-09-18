package wait

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWaitForCondition_should_ReturnNil_When_ConditionAlreadyTrue(t *testing.T) {
	err := WaitForCondition(func() bool { return true }, FastWaitConfig())
	if err != nil {
		t.Fatalf("WaitForCondition returned %v for an always-true condition", err)
	}
}

func TestWaitForCondition_should_ReturnTimeoutError_When_ConditionNeverTrue(t *testing.T) {
	freezeLoadFactor(t, 1.0)

	err := WaitForCondition(func() bool { return false }, WaitConfig{
		Timeout: 30 * time.Millisecond, PollInterval: 5 * time.Millisecond, Description: "the widget",
	})
	if err == nil {
		t.Fatal("expected a timeout error for an always-false condition")
	}
	if !strings.Contains(err.Error(), "the widget") {
		t.Errorf("error %q must include the config.Description", err.Error())
	}
}

func TestWaitForCondition_should_NoteScaling_When_TimeoutScaleOverrideApplies(t *testing.T) {
	freezeLoadFactor(t, 1.0)
	t.Setenv("STAPLER_SQUAD_TEST_TIMEOUT_SCALE", "3")

	err := WaitForCondition(func() bool { return false }, WaitConfig{
		Timeout: 10 * time.Millisecond, PollInterval: 5 * time.Millisecond, Description: "scaled widget",
	})
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if !strings.Contains(err.Error(), "scaled to") {
		t.Errorf("error %q must call out that the timeout was scaled, so a reader doesn't mistake it for the literal configured bound", err.Error())
	}
}

func TestWaitForConditionWithError_should_ReturnNil_When_ConditionBecomesTrue(t *testing.T) {
	calls := 0
	err := WaitForConditionWithError(func() (bool, error) {
		calls++
		return calls >= 2, nil
	}, FastWaitConfig())
	if err != nil {
		t.Fatalf("WaitForConditionWithError returned %v", err)
	}
}

func TestWaitForConditionWithError_should_IncludeLastError_When_TimingOut(t *testing.T) {
	freezeLoadFactor(t, 1.0)
	sentinel := errors.New("boom")

	err := WaitForConditionWithError(func() (bool, error) {
		return false, sentinel
	}, WaitConfig{Timeout: 20 * time.Millisecond, PollInterval: 5 * time.Millisecond, Description: "flaky op"})
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if !strings.Contains(err.Error(), sentinel.Error()) {
		t.Errorf("error %q must surface the last condition error for debuggability", err.Error())
	}
}
