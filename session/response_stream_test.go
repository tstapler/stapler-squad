package session

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestNewResponseStream(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", writer, buffer)

	rs := NewResponseStream("test-session", ptyAccess)
	if rs == nil {
		t.Fatal("NewResponseStream() returned nil")
	}

	if rs.sessionName != "test-session" {
		t.Errorf("Session name = %q, expected %q", rs.sessionName, "test-session")
	}

	if rs.bufferSize != 100 {
		t.Errorf("Buffer size = %d, expected 100", rs.bufferSize)
	}

	if rs.IsStarted() {
		t.Error("Stream should not be started initially")
	}
}

func TestNewResponseStreamWithBuffer(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", writer, buffer)

	rs := NewResponseStreamWithBuffer("test-session", ptyAccess, 500)
	if rs.bufferSize != 500 {
		t.Errorf("Buffer size = %d, expected 500", rs.bufferSize)
	}
}

func TestResponseStream_StartAndStop(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	ctx := context.Background()
	if err := rs.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if !rs.IsStarted() {
		t.Error("Stream should be started after Start()")
	}

	// Give the stream goroutine time to start
	time.Sleep(50 * time.Millisecond)

	if err := rs.Stop(); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}

	if rs.IsStarted() {
		t.Error("Stream should not be started after Stop()")
	}
}

func TestResponseStream_DoubleStart(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	ctx := context.Background()
	if err := rs.Start(ctx); err != nil {
		t.Fatalf("First Start() failed: %v", err)
	}

	// Second start should fail
	err = rs.Start(ctx)
	if err == nil {
		t.Error("Second Start() should fail")
	}

	rs.Stop()
}

func TestResponseStream_Subscribe(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	ch, err := rs.Subscribe("subscriber-1")
	if err != nil {
		t.Fatalf("Subscribe() failed: %v", err)
	}

	if ch == nil {
		t.Fatal("Subscribe() returned nil channel")
	}

	if rs.GetSubscriberCount() != 1 {
		t.Errorf("Subscriber count = %d, expected 1", rs.GetSubscriberCount())
	}
}

func TestResponseStream_DuplicateSubscribe(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	_, err = rs.Subscribe("subscriber-1")
	if err != nil {
		t.Fatalf("First Subscribe() failed: %v", err)
	}

	// Second subscribe with same ID should fail
	_, err = rs.Subscribe("subscriber-1")
	if err == nil {
		t.Error("Duplicate Subscribe() should fail")
	}
}

func TestResponseStream_Unsubscribe(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	ch, err := rs.Subscribe("subscriber-1")
	if err != nil {
		t.Fatalf("Subscribe() failed: %v", err)
	}

	if err := rs.Unsubscribe("subscriber-1"); err != nil {
		t.Fatalf("Unsubscribe() failed: %v", err)
	}

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("Channel should be closed after Unsubscribe()")
	}

	if rs.GetSubscriberCount() != 0 {
		t.Errorf("Subscriber count = %d, expected 0", rs.GetSubscriberCount())
	}
}

func TestResponseStream_UnsubscribeNonExistent(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	err = rs.Unsubscribe("nonexistent")
	if err == nil {
		t.Error("Unsubscribe() of non-existent subscriber should fail")
	}
}

func TestResponseStream_Streaming(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	// Subscribe before starting
	ch, err := rs.Subscribe("subscriber-1")
	if err != nil {
		t.Fatalf("Subscribe() failed: %v", err)
	}

	// Start streaming
	ctx := context.Background()
	if err := rs.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer rs.Stop()

	// Write some data to the PTY
	testData := []byte("test output\n")
	_, err = writer.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}

	// Wait for the data to be received
	select {
	case chunk := <-ch:
		if chunk.Error != nil {
			t.Errorf("Received error chunk: %v", chunk.Error)
		}
		if len(chunk.Data) == 0 {
			t.Error("Received empty data chunk")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for data chunk")
	}
}

func TestResponseStream_MultipleSubscribers(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	// Subscribe multiple subscribers
	ch1, _ := rs.Subscribe("subscriber-1")
	ch2, _ := rs.Subscribe("subscriber-2")
	ch3, _ := rs.Subscribe("subscriber-3")

	if rs.GetSubscriberCount() != 3 {
		t.Errorf("Subscriber count = %d, expected 3", rs.GetSubscriberCount())
	}

	// Start streaming
	ctx := context.Background()
	if err := rs.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer rs.Stop()

	// Write data
	testData := []byte("broadcast test\n")
	writer.Write(testData)

	// All subscribers should receive the data
	timeout := time.After(2 * time.Second)
	received := 0

	for received < 3 {
		select {
		case <-ch1:
			received++
		case <-ch2:
			received++
		case <-ch3:
			received++
		case <-timeout:
			t.Fatalf("Timeout: only %d/3 subscribers received data", received)
		}
	}
}

func TestResponseStream_GetSubscriberIDs(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	rs.Subscribe("sub-1")
	rs.Subscribe("sub-2")

	ids := rs.GetSubscriberIDs()
	if len(ids) != 2 {
		t.Errorf("GetSubscriberIDs() returned %d IDs, expected 2", len(ids))
	}
}

func TestResponseStream_GetSubscriberInfo(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	beforeSubscribe := time.Now()
	rs.Subscribe("sub-1")
	afterSubscribe := time.Now()

	created, exists := rs.GetSubscriberInfo("sub-1")
	if !exists {
		t.Error("GetSubscriberInfo() should return exists=true for existing subscriber")
	}

	if created.Before(beforeSubscribe) || created.After(afterSubscribe) {
		t.Error("Subscriber creation time is out of expected range")
	}

	_, exists = rs.GetSubscriberInfo("nonexistent")
	if exists {
		t.Error("GetSubscriberInfo() should return exists=false for non-existent subscriber")
	}
}

func TestResponseStream_SetBufferSize(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	rs.SetBufferSize(200)
	if rs.GetBufferSize() != 200 {
		t.Errorf("Buffer size = %d, expected 200", rs.GetBufferSize())
	}
}

func TestResponseStream_StopWithoutStart(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	err = rs.Stop()
	if err == nil {
		t.Error("Stop() without Start() should fail")
	}
}

func TestResponseStream_ContextCancellation(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	ch, _ := rs.Subscribe("sub-1")

	// Create a context with cancellation
	ctx, cancel := context.WithCancel(context.Background())

	if err := rs.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Give the stream time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel the context
	cancel()

	// Channel should be closed after context cancellation
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Channel should be closed after context cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for channel to close")
	}
}

func TestResponseStream_PTYClosed(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	ch, _ := rs.Subscribe("sub-1")

	ctx := context.Background()
	if err := rs.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Give the stream time to start
	time.Sleep(50 * time.Millisecond)

	// Close the PTY to simulate EOF
	writer.Close()
	reader.Close()

	// Channel should be closed after PTY closure
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Channel should be closed after PTY closure")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for channel to close after PTY closure")
	}
}

func TestResponseStream_NilPTYAccess(t *testing.T) {
	rs := NewResponseStream("test-session", nil)

	ctx := context.Background()
	err := rs.Start(ctx)
	if err == nil {
		t.Error("Start() with nil PTY access should fail")
	}
}

func TestResponseStream_ClosedPTYAccess(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	// Close PTY access
	ptyAccess.Close()

	rs := NewResponseStream("test-session", ptyAccess)

	ch, _ := rs.Subscribe("sub-1")

	ctx := context.Background()
	if err := rs.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Channel should close quickly since PTY is closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Channel should be closed for closed PTY")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for channel to close")
	}

	rs.Stop()
}

func TestResponseStream_StreamingWritesToBuffer(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	ctx := context.Background()
	if err := rs.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer rs.Stop()

	// Write some data
	testData := []byte("buffer test data")
	writer.Write(testData)

	// Wait for data to be processed
	time.Sleep(200 * time.Millisecond)

	// Check that data was written to the circular buffer
	bufferContents := buffer.GetAll()
	if len(bufferContents) == 0 {
		t.Error("Buffer should contain streamed data")
	}
}

func Benchmark_ResponseStream_Broadcast(b *testing.B) {
	reader, writer, _ := mockPTY()
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	// Add multiple subscribers
	for i := 0; i < 10; i++ {
		rs.Subscribe(string(rune('a' + i)))
	}

	chunk := ResponseChunk{
		Data:      []byte("benchmark data"),
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rs.broadcast(chunk)
	}
}

func Benchmark_ResponseStream_Subscribe(b *testing.B) {
	reader, writer, _ := mockPTY()
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rs := NewResponseStream("test-session", ptyAccess)
		rs.Subscribe("subscriber")
	}
}

// Helper to drain a channel without blocking
func drainChannel(ch <-chan ResponseChunk, timeout time.Duration) {
	done := time.After(timeout)
	for {
		select {
		case <-ch:
			// Drain
		case <-done:
			return
		}
	}
}

func TestResponseStream_HighThroughput(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024 * 10) // Larger buffer
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStreamWithBuffer("test-session", ptyAccess, 1000) // Large buffer

	ch, _ := rs.Subscribe("sub-1")

	ctx := context.Background()
	if err := rs.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer rs.Stop()

	// Start a goroutine to drain the channel
	go drainChannel(ch, 5*time.Second)

	// Write a lot of data quickly
	for i := 0; i < 100; i++ {
		writer.Write([]byte("data chunk\n"))
	}

	// Give time for processing
	time.Sleep(500 * time.Millisecond)

	// Test passes if we don't deadlock or panic
}

func TestResponseStream_ReadTimeout(t *testing.T) {
	// Create a PTY that will timeout on reads
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer func() {
		// Suppress expected errors during cleanup
		_ = reader.Close()
		_ = writer.Close()
	}()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	ctx := context.Background()
	if err := rs.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Let it run for a bit with no data (should handle read timeouts gracefully)
	time.Sleep(300 * time.Millisecond)

	// Should be able to stop without issues
	if err := rs.Stop(); err != nil {
		t.Errorf("Stop() failed: %v", err)
	}
}

func TestResponseStream_EmptyData(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	ch, _ := rs.Subscribe("sub-1")

	ctx := context.Background()
	if err := rs.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer rs.Stop()

	// Write empty data (should be ignored by read loop)
	writer.Write([]byte{})

	// Write actual data to verify stream still works
	writer.Write([]byte("real data"))

	// Should receive the real data
	select {
	case chunk := <-ch:
		if len(chunk.Data) == 0 {
			t.Error("Should not receive empty chunks")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for data")
	}
}

func TestResponseStream_ConcurrentSubscribeUnsubscribe(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	// Concurrently subscribe and unsubscribe
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			subID := string(rune('a' + id))
			if _, err := rs.Subscribe(subID); err == nil {
				time.Sleep(10 * time.Millisecond)
				rs.Unsubscribe(subID)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// All subscribers should be gone
	if rs.GetSubscriberCount() != 0 {
		t.Errorf("Subscriber count = %d, expected 0 after concurrent operations", rs.GetSubscriberCount())
	}
}

func TestResponseStream_NilReaderInPTY(t *testing.T) {
	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", nil, buffer) // nil PTY

	rs := NewResponseStream("test-session", ptyAccess)

	ctx := context.Background()
	if err := rs.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Should handle nil PTY gracefully (stream loop will detect and exit)
	time.Sleep(200 * time.Millisecond)

	rs.Stop()
}

func TestResponseStream_PTYReaderReturnsEOF(t *testing.T) {
	// Use an already-closed pipe to simulate EOF
	r, w, _ := os.Pipe()
	r.Close()
	w.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", r, buffer)

	rs := NewResponseStream("test-session", ptyAccess)

	ch, _ := rs.Subscribe("sub-1")

	ctx := context.Background()
	if err := rs.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Channel should close due to EOF
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Channel should be closed after EOF")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for channel to close after EOF")
	}
}
