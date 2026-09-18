package log

import (
	"strings"
	"sync"
	"testing"
)

// TestRedirectLogger_CapturesOutputWithPrefix is the functional-correctness
// check: a known message written through the redirected logger must show up
// in the returned buffer with the expected prefix.
func TestRedirectLogger_CapturesOutputWithPrefix(t *testing.T) {
	buf := RedirectLogger(t, ErrorLog, SetErrorLogForTest, "ERROR: ")

	ErrorLog().Println("something went wrong")

	got := buf.String()
	if !strings.Contains(got, "ERROR: ") || !strings.Contains(got, "something went wrong") {
		t.Errorf("captured output missing prefix/message: %q", got)
	}
}

// TestRedirectLogger_RestoresOriginalLoggerOnCleanup proves the pre-test
// logger is back in place once the redirecting test's cleanup has run —
// exercised via a sub-test so its t.Cleanup fires before this test asserts.
func TestRedirectLogger_RestoresOriginalLoggerOnCleanup(t *testing.T) {
	before := ErrorLog()

	t.Run("redirect", func(t *testing.T) {
		RedirectLogger(t, ErrorLog, SetErrorLogForTest, "ERROR: ")
		if ErrorLog() == before {
			t.Fatal("expected ErrorLog to be redirected during the sub-test")
		}
	})

	if ErrorLog() != before {
		t.Errorf("ErrorLog was not restored after cleanup: got %p, want original %p", ErrorLog(), before)
	}
}

// TestRedirectLogger_NoRaceUnderConcurrentWritersAndReader is the direct
// regression test for the reported bug pattern: N goroutines hammer the
// redirected logger with Printf while the test goroutine concurrently reads
// buf.String(), all under -race.
func TestRedirectLogger_NoRaceUnderConcurrentWritersAndReader(t *testing.T) {
	buf := RedirectLogger(t, ErrorLog, SetErrorLogForTest, "ERROR: ")

	const goroutines = 8
	const iterations = 200

	var writers sync.WaitGroup
	stop := make(chan struct{})
	readerDone := make(chan struct{})

	writers.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer writers.Done()
			for j := 0; j < iterations; j++ {
				ErrorLog().Printf("writer %d iteration %d", n, j)
			}
		}(i)
	}

	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stop:
				return
			default:
				_ = buf.String()
				_ = buf.Len()
			}
		}
	}()

	writers.Wait()
	close(stop)
	<-readerDone
}
