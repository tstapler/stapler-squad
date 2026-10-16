package streamhub_test

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// fakeTransport is a minimal Transport implementation used to prove the
// interface has exactly the two-method shape Story 1.1.1 requires — this
// test fails to compile if Transport grows an extra required method.
type fakeTransport struct{}

func (fakeTransport) Send(data []byte) error { return nil }
func (fakeTransport) Close() error           { return nil }

func TestTransport_should_BeSatisfiedByMinimalTwoMethodImplementation_When_SendAndCloseAreDefined(t *testing.T) {
	var tr streamhub.Transport = fakeTransport{}
	if err := tr.Send([]byte("data")); err != nil {
		t.Fatalf("Send() returned unexpected error: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("Close() returned unexpected error: %v", err)
	}
}

func TestNewSubscriberID_should_ReturnDistinctValues_When_CalledTwice(t *testing.T) {
	first := streamhub.NewSubscriberID()
	second := streamhub.NewSubscriberID()

	if first == "" || second == "" {
		t.Fatalf("expected non-empty SubscriberID values, got %q and %q", first, second)
	}
	if first == second {
		t.Fatalf("expected distinct SubscriberID values, both were %q", first)
	}
}

func TestSubscriberCapability_should_ExposeCanResizeAndCanWriteFields_When_Constructed(t *testing.T) {
	capability := streamhub.SubscriberCapability{CanResize: true, CanWrite: false}

	if !capability.CanResize {
		t.Fatal("expected CanResize to be true")
	}
	if capability.CanWrite {
		t.Fatal("expected CanWrite to be false")
	}
}

func TestHubLifecycleState_should_HaveExactlyFourDistinctValues_When_AllConstantsAreCompared(t *testing.T) {
	states := []streamhub.HubLifecycleState{
		streamhub.HubStarting,
		streamhub.HubActive,
		streamhub.HubDraining,
		streamhub.HubTornDown,
	}

	seen := make(map[streamhub.HubLifecycleState]bool, len(states))
	for _, s := range states {
		if seen[s] {
			t.Fatalf("HubLifecycleState value %v is not distinct from another constant", s)
		}
		seen[s] = true
	}
	if len(seen) != 4 {
		t.Fatalf("expected exactly 4 distinct HubLifecycleState values, got %d", len(seen))
	}
}

func TestStreamPath_should_HaveExactlyTwoDistinctValues_When_AllConstantsAreCompared(t *testing.T) {
	paths := []streamhub.StreamPath{
		streamhub.PathLegacyPerConnection,
		streamhub.PathHubOwned,
	}

	if paths[0] == paths[1] {
		t.Fatalf("expected PathLegacyPerConnection and PathHubOwned to be distinct, both were %v", paths[0])
	}
}
