package sessionstream

import (
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/server/events"
)

func TestDeployID_StampedOnSessionEvents(t *testing.T) {
	t.Parallel()
	got := convertEventToProto(&events.Event{Timestamp: time.Now(), Seq: 7})
	if got.GetDeployId() != events.DeployID() || got.GetSeq() != 7 {
		t.Fatalf("session event: deploy_id=%q seq=%d, want %q / 7", got.GetDeployId(), got.GetSeq(), events.DeployID())
	}
}
