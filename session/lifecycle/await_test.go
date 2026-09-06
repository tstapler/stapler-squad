package lifecycle_test

import (
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/lifecycle"
)

func TestAwaitBounded_ClosesInTime_ReturnsTrue(t *testing.T) {
	done := make(chan struct{})
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(done)
	}()

	start := time.Now()
	got := lifecycle.AwaitBounded(done, 200*time.Millisecond)
	elapsed := time.Since(start)

	if !got {
		t.Fatalf("AwaitBounded() = false, want true")
	}
	if elapsed >= 200*time.Millisecond {
		t.Fatalf("AwaitBounded() took %v, expected well under 200ms", elapsed)
	}
}

func TestAwaitBounded_NeverCloses_ReturnsFalseAfterBound(t *testing.T) {
	done := make(chan struct{})

	start := time.Now()
	got := lifecycle.AwaitBounded(done, 50*time.Millisecond)
	elapsed := time.Since(start)

	if got {
		t.Fatalf("AwaitBounded() = true, want false")
	}
	if elapsed < 50*time.Millisecond || elapsed >= 500*time.Millisecond {
		t.Fatalf("AwaitBounded() took %v, want within [50ms, 500ms)", elapsed)
	}
}
