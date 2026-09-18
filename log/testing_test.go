package log

import (
	stdlog "log"
	"strings"
	"sync"
	"testing"
)

// TestRedirectLogger_CapturesOutputWithPrefix is the functional-correctness
// check: a known message written through the redirected logger must show up
// in the returned buffer with the expected prefix.
func TestRedirectLogger_CapturesOutputWithPrefix(t *testing.T) {
	logger := stdlog.New(stdlog.Writer(), "orig: ", stdlog.LstdFlags)

	buf := RedirectLogger(t, logger, "TEST: ")
	logger.Println("something went wrong")

	got := buf.String()
	if !strings.Contains(got, "TEST: ") || !strings.Contains(got, "something went wrong") {
		t.Errorf("captured output missing prefix/message: %q", got)
	}
}

// TestRedirectLogger_CapturesWritesFromPreExistingReference verifies that a
// caller already holding a reference to logger (e.g. from an accessor like
// ErrorLog(), obtained before RedirectLogger is called) still has its
// writes land in the buffer. A swap-based implementation that reassigned a
// package-level logger *variable* instead of mutating this exact instance
// would fail this — the pre-existing reference would keep writing to the
// old destination, unaffected by the swap.
func TestRedirectLogger_CapturesWritesFromPreExistingReference(t *testing.T) {
	logger := stdlog.New(stdlog.Writer(), "orig: ", stdlog.LstdFlags)
	preRedirectRef := logger // captured before RedirectLogger is called

	buf := RedirectLogger(t, logger, "TEST: ")
	preRedirectRef.Println("written through the pre-existing reference")

	got := buf.String()
	if !strings.Contains(got, "written through the pre-existing reference") {
		t.Errorf("write through pre-redirect reference did not land in buffer: %q", got)
	}
}

// TestSyncBuffer_LenMatchesWrittenByteCount verifies Len() reports the
// actual number of bytes written, not just that it doesn't race (which
// TestRedirectLogger_NoRaceUnderConcurrentWritersAndReader already covers).
func TestSyncBuffer_LenMatchesWrittenByteCount(t *testing.T) {
	buf := &SyncBuffer{}
	const msg = "hello, sync buffer"
	n, err := buf.Write([]byte(msg))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != len(msg) {
		t.Errorf("Write returned n=%d, want %d", n, len(msg))
	}
	if got := buf.Len(); got != len(msg) {
		t.Errorf("Len() = %d, want %d", got, len(msg))
	}
}

// TestRedirectLogger_RestoresOriginalLoggerOnCleanup proves the logger's
// pre-test output/prefix/flags are back in place once the redirecting
// test's cleanup has run — exercised via a sub-test so its t.Cleanup fires
// before this test asserts.
func TestRedirectLogger_RestoresOriginalLoggerOnCleanup(t *testing.T) {
	logger := stdlog.New(stdlog.Writer(), "orig: ", stdlog.LstdFlags)
	origOutput := logger.Writer()
	origPrefix := logger.Prefix()
	origFlags := logger.Flags()

	t.Run("redirect", func(t *testing.T) {
		RedirectLogger(t, logger, "TEST: ")
		if logger.Prefix() != "TEST: " {
			t.Fatalf("logger was not redirected: prefix = %q, want %q", logger.Prefix(), "TEST: ")
		}
	})

	if logger.Writer() != origOutput || logger.Prefix() != origPrefix || logger.Flags() != origFlags {
		t.Errorf("logger not restored after cleanup: prefix=%q flags=%d, want prefix=%q flags=%d",
			logger.Prefix(), logger.Flags(), origPrefix, origFlags)
	}
}

// TestRedirectLogger_NoRaceUnderConcurrentWritersAndReader is the direct
// regression test for the reported bug pattern: N goroutines hammer the
// redirected logger with Printf while the test goroutine concurrently reads
// buf.String(), all under -race.
func TestRedirectLogger_NoRaceUnderConcurrentWritersAndReader(t *testing.T) {
	logger := stdlog.New(stdlog.Writer(), "orig: ", 0)
	buf := RedirectLogger(t, logger, "RACE: ")

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
				logger.Printf("writer %d iteration %d", n, j)
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
