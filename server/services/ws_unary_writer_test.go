package services

import "testing"

func TestUnaryWriter_enqueueResponse_should_RejectTailsOverQueuedByteLimit(t *testing.T) {
	w := newUnaryWriter(nil)
	payload := make([]byte, 32<<20)
	var err error
	accepted := 0
	for i := 0; i < 5 && err == nil; i++ {
		_, err = w.enqueueResponse(uint32(i+1), unaryResponseMeta{}, payload, nil)
		if err == nil {
			accepted++
		}
	}
	if err != errUnaryWriterOverflow {
		t.Fatalf("want errUnaryWriterOverflow after %d accepted, got %v", accepted, err)
	}
	if accepted != 2 {
		t.Fatalf("accepted = %d, want 2 (64 MiB cap, ~32 MiB tails)", accepted)
	}
	w.drop(1)
	w.drop(2)
	if w.queued != 0 {
		t.Fatalf("queued = %d after drop, want 0", w.queued)
	}
}
