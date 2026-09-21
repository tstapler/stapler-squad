package lifecycle_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/tstapler/stapler-squad/session/lifecycle"
)

func TestAwaitBounded_ClosesInTime_ReturnsTrue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			time.Sleep(10 * time.Millisecond)
			close(done)
		}()

		got := lifecycle.AwaitBounded(done, 200*time.Millisecond)

		if !got {
			t.Fatalf("AwaitBounded() = false, want true")
		}
	})
}

func TestAwaitBounded_NeverCloses_ReturnsFalseAfterBound(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		done := make(chan struct{})

		got := lifecycle.AwaitBounded(done, 50*time.Millisecond)

		if got {
			t.Fatalf("AwaitBounded() = true, want false")
		}
	})
}
