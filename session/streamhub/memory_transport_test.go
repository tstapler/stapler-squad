package streamhub_test

import (
	"errors"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

func TestMemoryTransport_should_SatisfyTransportInterface_When_SendAndCloseAreCalled(t *testing.T) {
	var tr streamhub.Transport = streamhub.NewMemoryTransport()
	if err := tr.Send([]byte("data")); err != nil {
		t.Fatalf("Send() returned unexpected error: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("Close() returned unexpected error: %v", err)
	}
}

func TestMemoryTransport_should_RecordReceivedFrames_When_SendSucceeds(t *testing.T) {
	mt := streamhub.NewMemoryTransport()

	if err := mt.Send([]byte("output-1")); err != nil {
		t.Fatalf("Send() returned unexpected error: %v", err)
	}
	if err := mt.Send([]byte("output-2")); err != nil {
		t.Fatalf("Send() returned unexpected error: %v", err)
	}

	got := mt.ReceivedFrames()
	want := [][]byte{[]byte("output-1"), []byte("output-2")}
	if len(got) != len(want) {
		t.Fatalf("ReceivedFrames() = %v, want %v", got, want)
	}
	for i := range want {
		if string(got[i]) != string(want[i]) {
			t.Fatalf("ReceivedFrames()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestMemoryTransport_should_ReturnIndependentFrameCopies_When_CallerMutatesInputSlice(t *testing.T) {
	mt := streamhub.NewMemoryTransport()
	data := []byte("original")
	if err := mt.Send(data); err != nil {
		t.Fatalf("Send() returned unexpected error: %v", err)
	}
	data[0] = 'X'

	got := mt.ReceivedFrames()
	if string(got[0]) != "original" {
		t.Fatalf("expected ReceivedFrames() to be unaffected by later mutation of the input slice, got %q", got[0])
	}
}

func TestMemoryTransport_should_BlockSendUntilUnblockIsCalled_When_ConstructedWithBlockingSend(t *testing.T) {
	mt := streamhub.NewMemoryTransport(streamhub.WithBlockingSend())

	sendReturned := make(chan error, 1)
	go func() {
		sendReturned <- mt.Send([]byte("blocked"))
	}()

	select {
	case <-sendReturned:
		t.Fatal("expected Send to block until Unblock is called")
	case <-time.After(50 * time.Millisecond):
		// Still blocked, as expected.
	}

	mt.Unblock()

	select {
	case err := <-sendReturned:
		if err != nil {
			t.Fatalf("expected Send to return nil after Unblock, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expected Send to return promptly after Unblock")
	}
}

func TestMemoryTransport_should_UnblockPendingSend_When_CloseIsCalled(t *testing.T) {
	mt := streamhub.NewMemoryTransport(streamhub.WithBlockingSend())

	sendReturned := make(chan error, 1)
	go func() {
		sendReturned <- mt.Send([]byte("blocked"))
	}()

	time.Sleep(20 * time.Millisecond)
	if err := mt.Close(); err != nil {
		t.Fatalf("Close() returned unexpected error: %v", err)
	}

	select {
	case <-sendReturned:
		// Send unblocked, as expected.
	case <-time.After(time.Second):
		t.Fatal("expected Close to release a Send call blocked by WithBlockingSend")
	}
	if !mt.IsClosed() {
		t.Fatal("expected IsClosed() to report true after Close")
	}
}

func TestMemoryTransport_should_ReturnConfiguredError_When_ConstructedWithErrorSend(t *testing.T) {
	wantErr := errors.New("boom")
	mt := streamhub.NewMemoryTransport(streamhub.WithErrorSend(wantErr))

	if err := mt.Send([]byte("frame")); !errors.Is(err, wantErr) {
		t.Fatalf("Send() = %v, want %v", err, wantErr)
	}
	if got := mt.ReceivedFrames(); len(got) != 0 {
		t.Fatalf("expected no frames recorded when Send errors, got %v", got)
	}
}

func TestMemoryTransport_should_BeSafeToCloseMultipleTimes(t *testing.T) {
	mt := streamhub.NewMemoryTransport()
	if err := mt.Close(); err != nil {
		t.Fatalf("first Close() returned unexpected error: %v", err)
	}
	if err := mt.Close(); err != nil {
		t.Fatalf("second Close() returned unexpected error: %v", err)
	}
}

func TestMemoryTransport_should_AttachToHubAndReceiveBroadcastFrame(t *testing.T) {
	hub := streamhub.NewStreamHub("test-session", nil, streamhub.WithTeardownGrace(time.Hour))
	mt := streamhub.NewMemoryTransport()

	id := hub.AttachSubscriber(mt, streamhub.SubscriberCapability{})
	hub.Broadcast([]byte("output-1"))

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && len(mt.ReceivedFrames()) == 0 {
		time.Sleep(time.Millisecond)
	}

	got := mt.ReceivedFrames()
	if len(got) != 1 || string(got[0]) != "output-1" {
		t.Fatalf("ReceivedFrames() = %v, want [[]byte(\"output-1\")]", got)
	}

	hub.DetachSubscriber(id)
}
