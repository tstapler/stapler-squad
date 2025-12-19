package workspace

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNoOpLock(t *testing.T) {
	lock := NewNoOpLock()
	defer lock.Close()

	ctx := context.Background()

	// Acquire lock
	handle, err := lock.Acquire(ctx, "test-resource", 5*time.Second)
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}

	if handle == nil {
		t.Fatal("expected handle to be non-nil")
	}

	if !handle.IsValid() {
		t.Error("expected handle to be valid")
	}

	if handle.Resource() != "test-resource" {
		t.Errorf("expected resource 'test-resource', got '%s'", handle.Resource())
	}

	// Try to acquire same resource (should block or fail in real impl)
	// NoOpLock uses in-memory tracking
	_, acquired, err := lock.TryAcquire(ctx, "test-resource")
	if err != nil {
		t.Fatalf("try acquire failed: %v", err)
	}
	if acquired {
		t.Error("expected try acquire to fail for held resource")
	}

	// Release
	err = handle.Release()
	if err != nil {
		t.Fatalf("release failed: %v", err)
	}

	if handle.IsValid() {
		t.Error("expected handle to be invalid after release")
	}

	// Should be able to acquire again
	handle2, err := lock.Acquire(ctx, "test-resource", 5*time.Second)
	if err != nil {
		t.Fatalf("second acquire failed: %v", err)
	}
	handle2.Release()
}

func TestNoOpLockConcurrent(t *testing.T) {
	lock := NewNoOpLock()
	defer lock.Close()

	const numGoroutines = 10
	var wg sync.WaitGroup
	acquired := make(chan bool, numGoroutines)

	ctx := context.Background()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			handle, ok, err := lock.TryAcquire(ctx, "shared-resource")
			if err != nil {
				t.Errorf("try acquire error: %v", err)
				return
			}

			acquired <- ok
			if ok && handle != nil {
				time.Sleep(10 * time.Millisecond)
				handle.Release()
			}
		}()
	}

	wg.Wait()
	close(acquired)

	// Count how many acquired
	count := 0
	for ok := range acquired {
		if ok {
			count++
		}
	}

	// At least one should have acquired
	if count == 0 {
		t.Error("expected at least one goroutine to acquire the lock")
	}
}

func TestNoOpLockTimeout(t *testing.T) {
	lock := NewNoOpLock()
	defer lock.Close()

	ctx := context.Background()

	// Acquire lock
	handle1, err := lock.Acquire(ctx, "timeout-resource", time.Second)
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}

	// Try to acquire with timeout (should timeout)
	_, err = lock.Acquire(ctx, "timeout-resource", 50*time.Millisecond)
	if err == nil {
		t.Error("expected acquire to timeout")
	}

	handle1.Release()
}

func TestNoOpLockExtend(t *testing.T) {
	lock := NewNoOpLock()
	defer lock.Close()

	ctx := context.Background()

	handle, _ := lock.Acquire(ctx, "extend-test", time.Second)

	// Extend should be no-op (no error)
	err := handle.Extend(time.Minute)
	if err != nil {
		t.Errorf("extend should not error: %v", err)
	}

	handle.Release()
}

func TestNoOpLockDoubleRelease(t *testing.T) {
	lock := NewNoOpLock()
	defer lock.Close()

	ctx := context.Background()

	handle, _ := lock.Acquire(ctx, "double-release", time.Second)

	// First release
	err := handle.Release()
	if err != nil {
		t.Errorf("first release failed: %v", err)
	}

	// Second release should be no-op
	err = handle.Release()
	if err != nil {
		t.Errorf("second release should be no-op: %v", err)
	}
}
