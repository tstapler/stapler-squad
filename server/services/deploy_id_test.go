package services

import (
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/server/events"
)

func TestDeployID_StableAndStamped(t *testing.T) {
	first := DeployID()
	if first == "" {
		t.Fatal("deploy id must not be empty")
	}
	time.Sleep(time.Millisecond)
	if again := DeployID(); again != first {
		t.Fatalf("deploy id changed within a process: %q then %q", first, again)
	}
	got := convertEventToProto(&events.Event{Timestamp: time.Now(), Seq: 7})
	if got.GetDeployId() != DeployID() || got.GetSeq() != 7 {
		t.Fatalf("session event: deploy_id=%q seq=%d, want %q / 7", got.GetDeployId(), got.GetSeq(), DeployID())
	}
	if w := convertEventToWorkflowEvent(&events.Event{Timestamp: time.Now()}); w.GetDeployId() != DeployID() {
		t.Fatalf("workflow event deploy_id=%q", w.GetDeployId())
	}
}
