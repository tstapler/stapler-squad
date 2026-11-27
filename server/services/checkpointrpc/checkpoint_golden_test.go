package checkpointrpc

// Differential golden for the checkpoint RPCs, recorded before the package move;
// the body is unchanged by it. Regenerate with -update-checkpoint-golden.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/queue"
)

var updateCheckpointGolden = flag.Bool("update-checkpoint-golden", false, "rewrite testdata/checkpoint.golden")

func TestCheckpointGolden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var out []string
	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }

	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	add("nil %v", checkpointToProto(nil))
	add("full %s", svctest.CanonicalProto(t, checkpointToProto(&domain.Checkpoint{
		ID: "id", SessionID: "s", ParentID: "p", Label: "l", ScrollbackSeq: 7, ScrollbackPath: "sp",
		ClaudeConvUUID: "u", GitCommitSHA: "sha", Timestamp: at,
	})))

	storage := svctest.CreateTestStorage(t)
	bus := events.NewEventBus(8)
	t.Cleanup(bus.Close)
	svc := NewCheckpointService(storage, bus)
	inst := &instance.Instance{Title: "golden-cp", Path: "/tmp/golden", Status: domain.Paused, Program: "claude",
		Checkpoints: domain.CheckpointList{{ID: "cp-1", Label: "baseline", Timestamp: at}}}
	require.NoError(t, storage.AddInstance(inst))

	list := func(label, id string) {
		res, err := svc.ListCheckpoints(ctx, connect.NewRequest(&sessionv1.ListCheckpointsRequest{SessionId: id}))
		msg := ""
		if err == nil {
			msg = svctest.CanonicalProto(t, res.Msg)
		}
		add("list %s -> %s %s", label, msg, svctest.CanonicalError(err))
	}
	clearState := func(label, id string) {
		res, err := svc.ClearConversationState(ctx, connect.NewRequest(&sessionv1.ClearConversationStateRequest{Id: id}))
		msg := ""
		if err == nil {
			msg = svctest.CanonicalProto(t, res.Msg)
		}
		add("clear %s -> %s %s", label, msg, svctest.CanonicalError(err))
	}
	list("empty-id", "")
	list("unwired", "golden-cp")
	clearState("empty-id", "")
	clearState("unwired", "golden-cp")
	svc.SetLoadInstancesFn(func() ([]*instance.Instance, error) { return nil, errors.New("boom") })
	clearState("loader-error", "golden-cp")
	svc.SetLoadInstancesFn(func() ([]*instance.Instance, error) { return []*instance.Instance{inst}, nil })
	clearState("loader", "golden-cp")
	poller := session.NewReviewQueuePoller(queue.NewReviewQueue(), instance.NewInstanceStatusManager(), nil)
	poller.SetInstances([]*instance.Instance{inst})
	svc.SetPoller(poller)
	list("poller", "golden-cp")
	clearState("poller", "golden-cp")
	for _, req := range []*sessionv1.CreateCheckpointRequest{{}, {SessionId: "golden-cp"}, {SessionId: "nope", Label: "l"}} {
		_, err := svc.CreateCheckpoint(ctx, connect.NewRequest(req))
		add("create %q %q -> %s", req.SessionId, req.Label, svctest.CanonicalError(err))
	}

	svctest.CompareGolden(t, filepath.Join("testdata", "checkpoint.golden"), out, *updateCheckpointGolden)
}
