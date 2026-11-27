package events

import "testing"

func TestDeployID_StableWithinProcess(t *testing.T) {
	t.Parallel()
	first := DeployID()
	if first == "" {
		t.Fatal("deploy id must not be empty")
	}
	if again := DeployID(); again != first {
		t.Fatalf("deploy id changed within a process: %q then %q", first, again)
	}
}
