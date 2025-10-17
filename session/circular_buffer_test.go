package session

import (
	"bytes"
	"os"
	"sync"
	"testing"
)

func TestCircularBuffer_Write(t *testing.T) {
	cb := NewCircularBuffer(10)

	// Write some data
	data := []byte("hello")
	n, err := cb.Write(data)
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() returned %d, expected %d", n, len(data))
	}

	// Verify data is in buffer
	all := cb.GetAll()
	if !bytes.Equal(all, data) {
		t.Errorf("GetAll() returned %q, expected %q", all, data)
	}
}

func TestCircularBuffer_WriteOverflow(t *testing.T) {
	cb := NewCircularBuffer(5)

	// Write more data than buffer can hold
	data := []byte("1234567890")
	n, err := cb.Write(data)
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() returned %d, expected %d", n, len(data))
	}

	// Should only keep last 5 bytes
	all := cb.GetAll()
	expected := []byte("67890")
	if !bytes.Equal(all, expected) {
		t.Errorf("GetAll() returned %q, expected %q", all, expected)
	}
}

func TestCircularBuffer_WriteLargerThanBuffer(t *testing.T) {
	cb := NewCircularBuffer(5)

	// Write data larger than buffer
	data := []byte("abcdefghij")
	n, err := cb.Write(data)
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() returned %d, expected %d", n, len(data))
	}

	// Should only keep last 5 bytes
	all := cb.GetAll()
	expected := []byte("fghij")
	if !bytes.Equal(all, expected) {
		t.Errorf("GetAll() returned %q, expected %q", all, expected)
	}
}

func TestCircularBuffer_GetRecent(t *testing.T) {
	cb := NewCircularBuffer(20)

	// Write some data
	data := []byte("0123456789")
	cb.Write(data)

	// Get last 5 bytes
	recent := cb.GetRecent(5)
	expected := []byte("56789")
	if !bytes.Equal(recent, expected) {
		t.Errorf("GetRecent(5) returned %q, expected %q", recent, expected)
	}

	// Get more than available
	recent = cb.GetRecent(20)
	if !bytes.Equal(recent, data) {
		t.Errorf("GetRecent(20) returned %q, expected %q", recent, data)
	}

	// Get zero bytes
	recent = cb.GetRecent(0)
	if recent != nil {
		t.Errorf("GetRecent(0) should return nil, got %q", recent)
	}
}

func TestCircularBuffer_GetRecentAfterWrap(t *testing.T) {
	cb := NewCircularBuffer(10)

	// Write data that wraps around
	cb.Write([]byte("0123456789"))
	cb.Write([]byte("ABCDE"))

	// Should have: "56789ABCDE"
	recent := cb.GetRecent(5)
	expected := []byte("ABCDE")
	if !bytes.Equal(recent, expected) {
		t.Errorf("GetRecent(5) after wrap returned %q, expected %q", recent, expected)
	}

	all := cb.GetAll()
	expectedAll := []byte("56789ABCDE")
	if !bytes.Equal(all, expectedAll) {
		t.Errorf("GetAll() after wrap returned %q, expected %q", all, expectedAll)
	}
}

func TestCircularBuffer_GetAll(t *testing.T) {
	cb := NewCircularBuffer(10)

	// Empty buffer
	all := cb.GetAll()
	if all != nil {
		t.Errorf("GetAll() on empty buffer should return nil, got %q", all)
	}

	// Write some data
	data := []byte("test")
	cb.Write(data)

	all = cb.GetAll()
	if !bytes.Equal(all, data) {
		t.Errorf("GetAll() returned %q, expected %q", all, data)
	}
}

func TestCircularBuffer_Len(t *testing.T) {
	cb := NewCircularBuffer(10)

	if cb.Len() != 0 {
		t.Errorf("Len() on empty buffer should return 0, got %d", cb.Len())
	}

	cb.Write([]byte("test"))
	if cb.Len() != 4 {
		t.Errorf("Len() should return 4, got %d", cb.Len())
	}

	// Write more to overflow
	cb.Write([]byte("1234567"))
	if cb.Len() != 10 {
		t.Errorf("Len() after overflow should return 10, got %d", cb.Len())
	}
}

func TestCircularBuffer_Cap(t *testing.T) {
	cb := NewCircularBuffer(100)
	if cb.Cap() != 100 {
		t.Errorf("Cap() should return 100, got %d", cb.Cap())
	}
}

func TestCircularBuffer_Clear(t *testing.T) {
	cb := NewCircularBuffer(10)

	cb.Write([]byte("test"))
	cb.Clear()

	if cb.Len() != 0 {
		t.Errorf("Len() after Clear() should return 0, got %d", cb.Len())
	}

	all := cb.GetAll()
	if all != nil {
		t.Errorf("GetAll() after Clear() should return nil, got %q", all)
	}
}

func TestCircularBuffer_ConcurrentWrites(t *testing.T) {
	cb := NewCircularBuffer(1024)

	const numGoroutines = 10
	const writesPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				data := []byte{byte(id)}
				_, err := cb.Write(data)
				if err != nil {
					t.Errorf("Concurrent write failed: %v", err)
					return
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify buffer has some data
	if cb.Len() == 0 {
		t.Error("Buffer should contain data after concurrent writes")
	}
}

func TestCircularBuffer_ConcurrentReads(t *testing.T) {
	cb := NewCircularBuffer(1024)

	// Write some initial data
	cb.Write([]byte("initial data for testing"))

	const numGoroutines = 10
	const readsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < readsPerGoroutine; j++ {
				_ = cb.GetAll()
				_ = cb.GetRecent(10)
				_ = cb.Len()
			}
		}()
	}

	wg.Wait()
}

func TestCircularBuffer_ConcurrentReadWrite(t *testing.T) {
	cb := NewCircularBuffer(1024)

	var wg sync.WaitGroup

	// Writers
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cb.Write([]byte{byte(id)})
			}
		}(i)
	}

	// Readers
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = cb.GetAll()
			}
		}()
	}

	wg.Wait()
}

func TestCircularBuffer_WriteTo(t *testing.T) {
	cb := NewCircularBuffer(20)

	// Write some data
	data := []byte("test data")
	cb.Write(data)

	// Write to a buffer
	var buf bytes.Buffer
	n, err := cb.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo() failed: %v", err)
	}
	if n != int64(len(data)) {
		t.Errorf("WriteTo() returned %d bytes, expected %d", n, len(data))
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Errorf("WriteTo() wrote %q, expected %q", buf.Bytes(), data)
	}
}

func TestCircularBuffer_WriteToAfterWrap(t *testing.T) {
	cb := NewCircularBuffer(10)

	// Write data that wraps around
	cb.Write([]byte("0123456789"))
	cb.Write([]byte("ABC"))

	// Should have: "3456789ABC"
	var buf bytes.Buffer
	n, err := cb.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo() failed: %v", err)
	}
	expected := []byte("3456789ABC")
	if n != int64(len(expected)) {
		t.Errorf("WriteTo() returned %d bytes, expected %d", n, len(expected))
	}
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("WriteTo() wrote %q, expected %q", buf.Bytes(), expected)
	}
}

func TestCircularBuffer_DiskFallback(t *testing.T) {
	cb := NewCircularBuffer(10)

	// Enable disk fallback
	tmpDir := os.TempDir()
	err := cb.EnableDiskFallback(tmpDir)
	if err != nil {
		t.Fatalf("EnableDiskFallback() failed: %v", err)
	}

	// Disable and cleanup
	err = cb.DisableDiskFallback()
	if err != nil {
		t.Fatalf("DisableDiskFallback() failed: %v", err)
	}

	// Should be able to enable again
	err = cb.EnableDiskFallback(tmpDir)
	if err != nil {
		t.Fatalf("Second EnableDiskFallback() failed: %v", err)
	}

	// Cleanup
	cb.Close()
}

func TestCircularBuffer_Close(t *testing.T) {
	cb := NewCircularBuffer(10)

	// Enable disk fallback
	tmpDir := os.TempDir()
	err := cb.EnableDiskFallback(tmpDir)
	if err != nil {
		t.Fatalf("EnableDiskFallback() failed: %v", err)
	}

	// Close should cleanup disk file
	err = cb.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// Should be safe to close again
	err = cb.Close()
	if err != nil {
		t.Errorf("Second Close() failed: %v", err)
	}
}

func TestCircularBuffer_DefaultSize(t *testing.T) {
	cb := NewCircularBuffer(0)
	if cb.Cap() != DefaultBufferSize {
		t.Errorf("Default buffer size should be %d, got %d", DefaultBufferSize, cb.Cap())
	}

	cb = NewCircularBuffer(-1)
	if cb.Cap() != DefaultBufferSize {
		t.Errorf("Negative size should use default %d, got %d", DefaultBufferSize, cb.Cap())
	}
}
