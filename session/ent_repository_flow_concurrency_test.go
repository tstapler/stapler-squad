package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/flow"
)

// A write issued while a flow read is parked inside StreamEvents must commit
// without waiting on the read: the read holds no writer connection.
func TestFlowRead_should_NotBlockWriter_When_StreamEventsPausedOnBarrier(t *testing.T) {
	t.Parallel()
	repo := newFlowTestRepo(t)
	c := repo.GetEntClient()
	at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	item := seedFlowItem(t, c, "barrier")
	for i := 0; i < 4; i++ {
		seedFlowEvent(t, c, item, "ready", "in_progress", at.Add(time.Duration(i)*time.Minute), "")
	}

	inRead, release := make(chan struct{}), make(chan struct{})
	readDone := make(chan error, 1)
	go func() {
		first := true
		readDone <- NewEntFlowReader(c).StreamEvents(context.Background(),
			flow.EventRange{From: at.Add(-time.Hour), ChunkItems: 1},
			func([]flow.Event) error {
				if first {
					first = false
					close(inRead)
					<-release
				}
				return nil
			})
	}()
	<-inRead

	// The deadline only bounds a regression; a healthy run never waits on it.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	written := make(chan error, 1)
	go func() {
		_, err := c.BacklogStatusEvent.Create().SetItemID(item).SetFromStatus("in_progress").
			SetToStatus("review").SetTriggeredBy("agent").SetCreatedAt(at.Add(time.Hour)).Save(ctx)
		written <- err
	}()
	select {
	case err := <-written:
		require.NoError(t, err, "the write committed while the read was still paused")
	case <-ctx.Done():
		t.Fatal("write blocked behind a paused flow read")
	}

	close(release)
	require.NoError(t, <-readDone)
}
