package terminal

import (
	"bytes"
	"sync"
	"testing"
)

func TestLineRingBuffer_NewBuffer(t *testing.T) {
	rb := NewLineRingBuffer(10)
	if rb.Size() != 0 {
		t.Errorf("New buffer should be empty, got size %d", rb.Size())
	}
	if rb.Capacity() != 10 {
		t.Errorf("Expected capacity 10, got %d", rb.Capacity())
	}
}

func TestLineRingBuffer_AppendAndGet(t *testing.T) {
	rb := NewLineRingBuffer(3)

	// Append first line
	rb.Append([]byte("line1"))
	if rb.Size() != 1 {
		t.Errorf("Expected size 1, got %d", rb.Size())
	}

	line := rb.Get(0)
	if !bytes.Equal(line, []byte("line1")) {
		t.Errorf("Expected 'line1', got '%s'", string(line))
	}

	// Append more lines
	rb.Append([]byte("line2"))
	rb.Append([]byte("line3"))

	if rb.Size() != 3 {
		t.Errorf("Expected size 3, got %d", rb.Size())
	}

	// Verify all lines
	expectedLines := []string{"line1", "line2", "line3"}
	for i, expected := range expectedLines {
		line := rb.Get(i)
		if !bytes.Equal(line, []byte(expected)) {
			t.Errorf("Line %d: expected '%s', got '%s'", i, expected, string(line))
		}
	}
}

func TestLineRingBuffer_Wraparound(t *testing.T) {
	rb := NewLineRingBuffer(3)

	// Fill buffer
	rb.Append([]byte("line1"))
	rb.Append([]byte("line2"))
	rb.Append([]byte("line3"))

	// Append one more (should wrap around and overwrite line1)
	rb.Append([]byte("line4"))

	if rb.Size() != 3 {
		t.Errorf("Expected size 3 after wraparound, got %d", rb.Size())
	}

	// Should have lines 2, 3, 4
	expectedLines := []string{"line2", "line3", "line4"}
	for i, expected := range expectedLines {
		line := rb.Get(i)
		if !bytes.Equal(line, []byte(expected)) {
			t.Errorf("Line %d: expected '%s', got '%s'", i, expected, string(line))
		}
	}
}

func TestLineRingBuffer_SetAll(t *testing.T) {
	rb := NewLineRingBuffer(5)

	// Set initial lines
	lines := [][]byte{
		[]byte("line1"),
		[]byte("line2"),
		[]byte("line3"),
	}
	rb.SetAll(lines)

	if rb.Size() != 3 {
		t.Errorf("Expected size 3, got %d", rb.Size())
	}

	// Verify lines
	for i, expected := range lines {
		line := rb.Get(i)
		if !bytes.Equal(line, expected) {
			t.Errorf("Line %d: expected '%s', got '%s'", i, string(expected), string(line))
		}
	}
}

func TestLineRingBuffer_SetAllTruncation(t *testing.T) {
	rb := NewLineRingBuffer(3)

	// Set more lines than capacity (should keep only last 3)
	lines := [][]byte{
		[]byte("line1"),
		[]byte("line2"),
		[]byte("line3"),
		[]byte("line4"),
		[]byte("line5"),
	}
	rb.SetAll(lines)

	if rb.Size() != 3 {
		t.Errorf("Expected size 3 after truncation, got %d", rb.Size())
	}

	// Should have last 3 lines (3, 4, 5)
	expectedLines := []string{"line3", "line4", "line5"}
	for i, expected := range expectedLines {
		line := rb.Get(i)
		if !bytes.Equal(line, []byte(expected)) {
			t.Errorf("Line %d: expected '%s', got '%s'", i, expected, string(line))
		}
	}
}

func TestLineRingBuffer_GetOutOfBounds(t *testing.T) {
	rb := NewLineRingBuffer(3)
	rb.Append([]byte("line1"))

	// Test negative index
	line := rb.Get(-1)
	if line != nil {
		t.Error("Expected nil for negative index")
	}

	// Test index beyond size
	line = rb.Get(5)
	if line != nil {
		t.Error("Expected nil for index beyond size")
	}
}

func TestLineRingBuffer_GetAllLines(t *testing.T) {
	rb := NewLineRingBuffer(5)

	lines := [][]byte{
		[]byte("line1"),
		[]byte("line2"),
		[]byte("line3"),
	}
	rb.SetAll(lines)

	allLines := rb.GetAllLines()
	if len(allLines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(allLines))
	}

	for i, expected := range lines {
		if !bytes.Equal(allLines[i], expected) {
			t.Errorf("Line %d: expected '%s', got '%s'", i, string(expected), string(allLines[i]))
		}
	}
}

func TestLineRingBuffer_UpdateDimensions(t *testing.T) {
	rb := NewLineRingBuffer(3)

	// Add some lines
	rb.Append([]byte("line1"))
	rb.Append([]byte("line2"))
	rb.Append([]byte("line3"))

	// Resize to larger capacity
	rb.UpdateDimensions(5)

	if rb.Capacity() != 5 {
		t.Errorf("Expected capacity 5, got %d", rb.Capacity())
	}

	// Existing lines should be preserved
	if rb.Size() != 3 {
		t.Errorf("Expected size 3, got %d", rb.Size())
	}

	expectedLines := []string{"line1", "line2", "line3"}
	for i, expected := range expectedLines {
		line := rb.Get(i)
		if !bytes.Equal(line, []byte(expected)) {
			t.Errorf("Line %d: expected '%s', got '%s'", i, expected, string(line))
		}
	}
}

func TestLineRingBuffer_UpdateDimensionsSmaller(t *testing.T) {
	rb := NewLineRingBuffer(5)

	// Add 5 lines
	for i := 1; i <= 5; i++ {
		rb.Append([]byte("line" + string(rune('0'+i))))
	}

	// Resize to smaller capacity (should keep only last 3 lines)
	rb.UpdateDimensions(3)

	if rb.Capacity() != 3 {
		t.Errorf("Expected capacity 3, got %d", rb.Capacity())
	}

	if rb.Size() != 3 {
		t.Errorf("Expected size 3, got %d", rb.Size())
	}

	// Should have last 3 lines (line3, line4, line5)
	expectedLines := []string{"line3", "line4", "line5"}
	for i, expected := range expectedLines {
		line := rb.Get(i)
		if !bytes.Equal(line, []byte(expected)) {
			t.Errorf("Line %d: expected '%s', got '%s'", i, expected, string(line))
		}
	}
}

func TestLineRingBuffer_ThreadSafety(t *testing.T) {
	rb := NewLineRingBuffer(100)

	var wg sync.WaitGroup
	iterations := 1000

	// Start multiple writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				line := []byte("line from goroutine")
				rb.Append(line)
			}
		}(i)
	}

	// Start multiple readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = rb.Get(0)
				_ = rb.Size()
				_ = rb.GetAllLines()
			}
		}()
	}

	wg.Wait()

	// Buffer should have up to 100 lines
	if rb.Size() > 100 {
		t.Errorf("Buffer size exceeded capacity: %d", rb.Size())
	}
}

// Benchmark tests
func BenchmarkLineRingBuffer_Append(b *testing.B) {
	rb := NewLineRingBuffer(100)
	line := []byte("test line with some content")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Append(line)
	}
}

func BenchmarkLineRingBuffer_Get(b *testing.B) {
	rb := NewLineRingBuffer(100)
	for i := 0; i < 100; i++ {
		rb.Append([]byte("test line"))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rb.Get(i % 100)
	}
}

func BenchmarkLineRingBuffer_SetAll(b *testing.B) {
	lines := make([][]byte, 100)
	for i := range lines {
		lines[i] = []byte("test line with content")
	}

	rb := NewLineRingBuffer(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.SetAll(lines)
	}
}

// Comparison benchmarks: Ring buffer vs Slice
func BenchmarkSliceAppend(b *testing.B) {
	var lines [][]byte
	line := []byte("test line with some content")
	maxSize := 100

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lines = append(lines, line)
		if len(lines) > maxSize {
			lines = lines[1:] // Remove oldest
		}
	}
}

func BenchmarkSliceSetAll(b *testing.B) {
	newLines := make([][]byte, 100)
	for i := range newLines {
		newLines[i] = []byte("test line with content")
	}

	var lines [][]byte

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lines = newLines
		if len(lines) > 100 {
			lines = lines[len(lines)-100:]
		}
	}
}
