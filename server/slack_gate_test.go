package server

import (
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/session"
)

// T-PS-08 (Slack cell of the delivery matrix): through the real
// ReactiveQueueManager.OnItemAdded with the bus publish filter installed, a
// hidden session's routine reasons reach Slack zero times and its failure
// reasons once, exactly as before the gate existed. Slack is invoked from the
// review-queue path (not the bus), so this path is covered by the poller skip
// and suppressForHidden rather than the publish filter; the test pins that the
// presence of the gate does not change it.
func TestSlackNotifier_ShouldReceiveZeroRoutine_WhenHiddenReviewItemAdded(t *testing.T) {
	cases := []struct {
		reason    session.AttentionReason
		wantSlack int
	}{
		{session.ReasonTaskComplete, 0},
		{session.ReasonIdle, 0},
		{session.ReasonStale, 0},
		{session.ReasonTestsFailing, 1},
		{session.ReasonErrorState, 1},
	}
	for _, c := range cases {
		c := c
		t.Run(c.reason.String(), func(t *testing.T) {
			setTestSlackConfig(t, true, 0)
			mgr, poller, bus := newReactiveQueueTestSetup(t)

			gate := deliverygate.NewGate(deliverygate.WithFlagLoader(func() (deliverygate.FlagSettings, error) {
				return deliverygate.FlagSettings{Global: true}, nil
			}))
			gate.Flags().Reload()
			gate.Index().Replace([]deliverygate.Entry{{UUID: "slack-hidden-uuid", Title: "review:slack", Hidden: true, Kind: deliverygate.KindReview}})
			bus.SetPublishFilter(gate.PublishFilter())

			fake := &fakeSlackNotifierWiring{}
			mgr.SetSlackNotifier(fake)
			poller.SetInstances([]*session.Instance{{Title: "review:slack", UUID: "slack-hidden-uuid", Hidden: true}})

			mgr.OnItemAdded(&session.ReviewItem{
				SessionID: "review:slack", SessionName: "review:slack",
				Reason: c.reason, Priority: session.PriorityHigh, DetectedAt: time.Now(),
			})

			if notify, _ := fake.counts(); notify != c.wantSlack {
				t.Errorf("Slack NotifyReviewQueueItem calls = %d, want %d", notify, c.wantSlack)
			}
		})
	}
}
