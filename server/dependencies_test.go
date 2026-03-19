package server

import (
	"testing"
)

func TestBuildServiceDeps_RejectsNilCore(t *testing.T) {
	_, err := BuildServiceDeps(nil)
	if err == nil {
		t.Fatal("expected error for nil CoreDeps")
	}
}

func TestBuildServiceDeps_RejectsNilCoreFields(t *testing.T) {
	// CoreDeps with all nil fields should be rejected.
	core := &CoreDeps{}
	_, err := BuildServiceDeps(core)
	if err == nil {
		t.Fatal("expected error for CoreDeps with nil fields")
	}
}

func TestBuildRuntimeDeps_RejectsNilService(t *testing.T) {
	_, err := BuildRuntimeDeps(nil)
	if err == nil {
		t.Fatal("expected error for nil ServiceDeps")
	}
}
