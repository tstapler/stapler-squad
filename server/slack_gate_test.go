package server

import (
	"sync"
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

type recordingDispatcher struct {
	mu     sync.Mutex
	events []string
}

func (r *recordingDispatcher) Dispatch(eventType string, _ any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, eventType)
}

func (r *recordingDispatcher) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

// The Slack and webhook-callback sends are gated by the delivery gate itself,
// not by the legacy suppressForHidden predicate: the instance here is NOT
// Hidden (so the legacy check is inert, as after PR 2b) while the gate's index
// says hidden.
func TestQueueItemSends_ShouldFollowGate_WhenLegacyPredicateInert(t *testing.T) {
	cases := []struct {
		reason session.AttentionReason
		want   int
	}{
		{session.ReasonTaskComplete, 0},
		{session.ReasonIdle, 0},
		{session.ReasonTestsFailing, 1},
		{session.ReasonErrorState, 1},
	}
	for _, flagOn := range []bool{true, false} {
		for _, c := range cases {
			c, flagOn := c, flagOn
			t.Run(c.reason.String(), func(t *testing.T) {
				setTestSlackConfig(t, true, 0)
				mgr, poller, _ := newReactiveQueueTestSetup(t)
				gate := deliverygate.NewGate(deliverygate.WithFlagLoader(func() (deliverygate.FlagSettings, error) {
					return deliverygate.FlagSettings{Global: flagOn}, nil
				}))
				gate.Flags().Reload()
				gate.Index().Replace([]deliverygate.Entry{{UUID: "gate-hidden-uuid", Title: "review:gate", Hidden: true, Kind: deliverygate.KindReview}})
				mgr.SetQueueItemGate(gate.AllowQueueItem)

				fake := &fakeSlackNotifierWiring{}
				hook := &recordingDispatcher{}
				mgr.SetSlackNotifier(fake)
				mgr.SetCallbackDispatcher(hook)
				poller.SetInstances([]*session.Instance{{Title: "review:gate", UUID: "gate-hidden-uuid"}})

				mgr.OnItemAdded(&session.ReviewItem{
					SessionID: "review:gate", SessionName: "review:gate",
					Reason: c.reason, Priority: session.PriorityHigh, DetectedAt: time.Now(),
				})

				want := c.want
				if !flagOn {
					want = 1 // shadow mode: counted, never suppressed
				}
				if notify, _ := fake.counts(); notify != want {
					t.Errorf("Slack calls = %d, want %d", notify, want)
				}
				if got := hook.count(); got != want {
					t.Errorf("webhook dispatches = %d, want %d", got, want)
				}
			})
		}
	}
}
