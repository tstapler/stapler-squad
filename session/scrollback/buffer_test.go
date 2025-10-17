package scrollback

import (
	"sync"
	"testing"
	"time"
)

func TestNewCircularBuffer(t *testing.T) {
	buffer := NewCircularBuffer(10)
	if buffer == nil {
		t.Fatal("NewCircularBuffer returned nil")
	}
	if buffer.MaxSize() != 10 {
		t.Errorf("Expected max size 10, got %d", buffer.MaxSize())
	}
	if buffer.Size() != 0 {
		t.Errorf("Expected initial size 0, got %d", buffer.Size())
	}
}

func TestCircularBufferAppend(t *testing.T) {
	buffer := NewCircularBuffer(3)

	// Add first entry
	entry1, evicted := buffer.Append([]byte("first"))
	if evicted {
		t.Error("Expected no eviction on first append")
	}
	if entry1.Sequence != 1 {
		t.Errorf("Expected sequence 1, got %d", entry1.Sequence)
	}
	if string(entry1.Data) != "first" {
		t.Errorf("Expected data 'first', got '%s'", string(entry1.Data))
	}
	if buffer.Size() != 1 {
		t.Errorf("Expected size 1, got %d", buffer.Size())
	}

	// Add second entry
	entry2, evicted := buffer.Append([]byte("second"))
	if evicted {
		t.Error("Expected no eviction on second append")
	}
	if entry2.Sequence != 2 {
		t.Errorf("Expected sequence 2, got %d", entry2.Sequence)
	}
	if buffer.Size() != 2 {
		t.Errorf("Expected size 2, got %d", buffer.Size())
	}

	// Add third entry (buffer full)
	_, evicted = buffer.Append([]byte("third"))
	if evicted {
		t.Error("Expected no eviction on third append")
	}
	if buffer.Size() != 3 {
		t.Errorf("Expected size 3, got %d", buffer.Size())
	}

	// Add fourth entry (should evict first)
	entry4, evicted := buffer.Append([]byte("fourth"))
	if !evicted {
		t.Error("Expected eviction on fourth append")
	}
	if entry4.Sequence != 4 {
		t.Errorf("Expected sequence 4, got %d", entry4.Sequence)
	}
	if buffer.Size() != 3 {
		t.Errorf("Expected size to remain 3, got %d", buffer.Size())
	}
}

func TestCircularBufferGetLastN(t *testing.T) {
	buffer := NewCircularBuffer(5)

	// Add some entries
	buffer.Append([]byte("one"))
	buffer.Append([]byte("two"))
	buffer.Append([]byte("three"))
	buffer.Append([]byte("four"))

	// Get last 2 entries
	entries := buffer.GetLastN(2)
	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}
	if string(entries[0].Data) != "three" {
		t.Errorf("Expected 'three', got '%s'", string(entries[0].Data))
	}
	if string(entries[1].Data) != "four" {
		t.Errorf("Expected 'four', got '%s'", string(entries[1].Data))
	}

	// Get more entries than available
	entries = buffer.GetLastN(10)
	if len(entries) != 4 {
		t.Errorf("Expected 4 entries, got %d", len(entries))
	}
}

func TestCircularBufferGetRange(t *testing.T) {
	buffer := NewCircularBuffer(5)

	// Add entries
	buffer.Append([]byte("one"))   // seq 1
	buffer.Append([]byte("two"))   // seq 2
	buffer.Append([]byte("three")) // seq 3
	buffer.Append([]byte("four"))  // seq 4

	// Get from sequence 2
	entries := buffer.GetRange(2, 10)
	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(entries))
	}
	if string(entries[0].Data) != "two" {
		t.Errorf("Expected 'two', got '%s'", string(entries[0].Data))
	}
	if entries[0].Sequence != 2 {
		t.Errorf("Expected sequence 2, got %d", entries[0].Sequence)
	}

	// Get with limit
	entries = buffer.GetRange(1, 2)
	if len(entries) != 2 {
		t.Errorf("Expected 2 entries due to limit, got %d", len(entries))
	}
}

func TestCircularBufferOverflow(t *testing.T) {
	buffer := NewCircularBuffer(3)

	// Fill buffer beyond capacity
	buffer.Append([]byte("1"))
	buffer.Append([]byte("2"))
	buffer.Append([]byte("3"))
	buffer.Append([]byte("4")) // Overwrites "1"
	buffer.Append([]byte("5")) // Overwrites "2"

	// Should only have last 3 entries
	entries := buffer.GetAll()
	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(entries))
	}
	if string(entries[0].Data) != "3" {
		t.Errorf("Expected '3', got '%s'", string(entries[0].Data))
	}
	if string(entries[1].Data) != "4" {
		t.Errorf("Expected '4', got '%s'", string(entries[1].Data))
	}
	if string(entries[2].Data) != "5" {
		t.Errorf("Expected '5', got '%s'", string(entries[2].Data))
	}

	// Check sequences
	if entries[0].Sequence != 3 {
		t.Errorf("Expected sequence 3, got %d", entries[0].Sequence)
	}
	if entries[2].Sequence != 5 {
		t.Errorf("Expected sequence 5, got %d", entries[2].Sequence)
	}
}

func TestCircularBufferConcurrency(t *testing.T) {
	buffer := NewCircularBuffer(1000)
	numGoroutines := 10
	entriesPerGoroutine := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < entriesPerGoroutine; j++ {
				data := []byte{byte(id), byte(j)}
				buffer.Append(data)
			}
		}(i)
	}

	wg.Wait()

	// Verify total entries
	if buffer.Size() != numGoroutines*entriesPerGoroutine {
		t.Errorf("Expected %d entries, got %d", numGoroutines*entriesPerGoroutine, buffer.Size())
	}

	// Verify sequence numbers are monotonic
	entries := buffer.GetAll()
	for i := 1; i < len(entries); i++ {
		if entries[i].Sequence <= entries[i-1].Sequence {
			t.Errorf("Sequence numbers not monotonic at index %d: %d <= %d",
				i, entries[i].Sequence, entries[i-1].Sequence)
		}
	}
}

func TestCircularBufferClear(t *testing.T) {
	buffer := NewCircularBuffer(5)

	buffer.Append([]byte("one"))
	buffer.Append([]byte("two"))
	buffer.Append([]byte("three"))

	if buffer.Size() != 3 {
		t.Errorf("Expected size 3, got %d", buffer.Size())
	}

	buffer.Clear()

	if buffer.Size() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", buffer.Size())
	}

	entries := buffer.GetAll()
	if len(entries) != 0 {
		t.Errorf("Expected no entries after clear, got %d", len(entries))
	}

	// Verify we can append after clear
	buffer.Append([]byte("new"))
	if buffer.Size() != 1 {
		t.Errorf("Expected size 1 after appending to cleared buffer, got %d", buffer.Size())
	}
}

func TestCircularBufferDirtyFlag(t *testing.T) {
	buffer := NewCircularBuffer(5)

	if buffer.IsDirty() {
		t.Error("New buffer should not be dirty")
	}

	buffer.Append([]byte("data"))
	if !buffer.IsDirty() {
		t.Error("Buffer should be dirty after append")
	}

	buffer.MarkClean()
	if buffer.IsDirty() {
		t.Error("Buffer should not be dirty after MarkClean")
	}

	buffer.Clear()
	if !buffer.IsDirty() {
		t.Error("Buffer should be dirty after clear")
	}
}

func TestCircularBufferSequenceNumbers(t *testing.T) {
	buffer := NewCircularBuffer(3)

	if buffer.GetOldestSequence() != 0 {
		t.Errorf("Empty buffer should have oldest sequence 0, got %d", buffer.GetOldestSequence())
	}
	if buffer.GetNewestSequence() != 0 {
		t.Errorf("Empty buffer should have newest sequence 0, got %d", buffer.GetNewestSequence())
	}

	buffer.Append([]byte("one"))
	buffer.Append([]byte("two"))
	buffer.Append([]byte("three"))

	if buffer.GetOldestSequence() != 1 {
		t.Errorf("Expected oldest sequence 1, got %d", buffer.GetOldestSequence())
	}
	if buffer.GetNewestSequence() != 3 {
		t.Errorf("Expected newest sequence 3, got %d", buffer.GetNewestSequence())
	}

	// Overflow - oldest should change
	buffer.Append([]byte("four"))

	if buffer.GetOldestSequence() != 2 {
		t.Errorf("Expected oldest sequence 2 after overflow, got %d", buffer.GetOldestSequence())
	}
	if buffer.GetNewestSequence() != 4 {
		t.Errorf("Expected newest sequence 4, got %d", buffer.GetNewestSequence())
	}
}

func TestCircularBufferDataIsolation(t *testing.T) {
	buffer := NewCircularBuffer(5)

	// Original data
	data := []byte("mutable")
	buffer.Append(data)

	// Modify original
	data[0] = 'X'

	// Verify buffer has unmodified copy
	entries := buffer.GetAll()
	if string(entries[0].Data) != "mutable" {
		t.Errorf("Buffer data should be isolated, got '%s'", string(entries[0].Data))
	}
}

func TestCircularBufferTimestamps(t *testing.T) {
	buffer := NewCircularBuffer(5)

	before := time.Now()
	buffer.Append([]byte("test"))
	after := time.Now()

	entries := buffer.GetAll()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	ts := entries[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("Timestamp %v should be between %v and %v", ts, before, after)
	}
}

func BenchmarkCircularBufferAppend(b *testing.B) {
	buffer := NewCircularBuffer(10000)
	data := []byte("benchmark data entry")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer.Append(data)
	}
}

func BenchmarkCircularBufferGetLastN(b *testing.B) {
	buffer := NewCircularBuffer(10000)
	for i := 0; i < 10000; i++ {
		buffer.Append([]byte("entry"))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer.GetLastN(100)
	}
}

func BenchmarkCircularBufferConcurrentAppend(b *testing.B) {
	buffer := NewCircularBuffer(100000)
	data := []byte("concurrent benchmark data")

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buffer.Append(data)
		}
	})
}
