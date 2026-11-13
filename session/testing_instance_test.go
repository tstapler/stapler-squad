package session

import (
	"github.com/tstapler/stapler-squad/session/domain"
	"testing"
)

func TestNewPausedTestInstance_should_ReturnAStartedPausedInstance(t *testing.T) {
	t.Parallel()
	inst := NewPausedTestInstance("paused-helper")
	if inst.Title != "paused-helper" || inst.Status != domain.Paused || inst.Program != "claude" {
		t.Fatalf("unexpected instance: %+v", inst)
	}
	if !inst.Started() {
		t.Fatal("the instance must be marked started so Storage.AddInstance persists it")
	}
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	if err := storage.AddInstance(inst); err != nil {
		t.Fatalf("AddInstance: %v", err)
	}
}
