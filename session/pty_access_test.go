package session

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"
)

// mockPTY creates a pipe that simulates a PTY for testing
func mockPTY() (*os.File, *os.File, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	return reader, writer, nil
}

func TestPTYAccess_Write(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", writer, buffer)

	testData := []byte("test command\n")
	n, err := ptyAccess.Write(testData)
	if err != nil {
		t.Errorf("Write() failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Write() returned %d bytes, expected %d", n, len(testData))
	}

	// Read from the reader side to verify data was written
	readBuf := make([]byte, len(testData))
	n, err = reader.Read(readBuf)
	if err != nil {
		t.Errorf("Failed to read from PTY: %v", err)
	}
	if !bytes.Equal(readBuf, testData) {
		t.Errorf("Read data %q, expected %q", readBuf, testData)
	}
}

func TestPTYAccess_Read(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	// Write data to the writer side
	testData := []byte("test output\n")
	_, err = writer.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write to PTY: %v", err)
	}

	// Read from PTY access
	readBuf := make([]byte, len(testData))
	n, err := ptyAccess.Read(readBuf)
	if err != nil {
		t.Errorf("Read() failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Read() returned %d bytes, expected %d", n, len(testData))
	}
	if !bytes.Equal(readBuf, testData) {
		t.Errorf("Read data %q, expected %q", readBuf, testData)
	}
}

func TestPTYAccess_ConcurrentWrites(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", writer, buffer)

	// Test concurrent writes don't cause race conditions
	const numGoroutines = 10
	const writesPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Drain reader in background to prevent blocking
	go func() {
		buf := make([]byte, 1024)
		for {
			_, err := reader.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				data := []byte{byte(id)}
				_, err := ptyAccess.Write(data)
				if err != nil {
					t.Errorf("Concurrent write failed: %v", err)
					return
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestPTYAccess_ConcurrentReads(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	// Write data continuously in background
	stopWriting := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopWriting:
				return
			default:
				writer.Write([]byte("test\n"))
			}
		}
	}()

	// Test concurrent reads don't cause race conditions
	const numGoroutines = 5
	const readsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			buf := make([]byte, 32)
			for j := 0; j < readsPerGoroutine; j++ {
				_, err := ptyAccess.Read(buf)
				if err != nil && err != io.EOF {
					t.Errorf("Concurrent read failed: %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()
	close(stopWriting)
}

func TestPTYAccess_GetBuffer(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	// Write some data to the buffer
	testData := []byte("buffered output")
	buffer.Write(testData)

	// Get buffer contents
	contents := ptyAccess.GetBuffer()
	if !bytes.Equal(contents, testData) {
		t.Errorf("GetBuffer() returned %q, expected %q", contents, testData)
	}
}

func TestPTYAccess_GetRecentOutput(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)

	// Write some data to the buffer
	testData := []byte("1234567890")
	buffer.Write(testData)

	// Get recent 5 bytes
	recent := ptyAccess.GetRecentOutput(5)
	expected := []byte("67890")
	if !bytes.Equal(recent, expected) {
		t.Errorf("GetRecentOutput(5) returned %q, expected %q", recent, expected)
	}
}

func TestPTYAccess_UpdatePTY(t *testing.T) {
	reader1, writer1, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create first mock PTY: %v", err)
	}
	defer reader1.Close()
	defer writer1.Close()

	reader2, writer2, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create second mock PTY: %v", err)
	}
	defer reader2.Close()
	defer writer2.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", writer1, buffer)

	// Update to new PTY
	err = ptyAccess.UpdatePTY(writer2)
	if err != nil {
		t.Errorf("UpdatePTY() failed: %v", err)
	}

	// Write to the updated PTY
	testData := []byte("new pty test\n")
	n, err := ptyAccess.Write(testData)
	if err != nil {
		t.Errorf("Write() after UpdatePTY() failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Write() returned %d bytes, expected %d", n, len(testData))
	}

	// Verify data goes to new PTY
	readBuf := make([]byte, len(testData))
	n, err = reader2.Read(readBuf)
	if err != nil {
		t.Errorf("Failed to read from new PTY: %v", err)
	}
	if !bytes.Equal(readBuf, testData) {
		t.Errorf("Read data %q, expected %q", readBuf, testData)
	}
}

func TestPTYAccess_Close(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", writer, buffer)

	// Close PTY access
	err = ptyAccess.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	if !ptyAccess.IsClosed() {
		t.Error("IsClosed() returned false after Close()")
	}

	// Verify operations fail after close
	_, err = ptyAccess.Write([]byte("test"))
	if err == nil {
		t.Error("Write() should fail after Close()")
	}

	buf := make([]byte, 32)
	_, err = ptyAccess.Read(buf)
	if err == nil {
		t.Error("Read() should fail after Close()")
	}
}

func TestPTYAccess_ClosedUpdate(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", writer, buffer)

	// Close PTY access
	ptyAccess.Close()

	// Verify UpdatePTY fails after close
	reader2, writer2, _ := mockPTY()
	defer reader2.Close()
	defer writer2.Close()

	err = ptyAccess.UpdatePTY(writer2)
	if err == nil {
		t.Error("UpdatePTY() should fail after Close()")
	}
}

func TestPTYAccess_NilPTY(t *testing.T) {
	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", nil, buffer)

	// Verify operations fail with nil PTY
	_, err := ptyAccess.Write([]byte("test"))
	if err == nil {
		t.Error("Write() should fail with nil PTY")
	}

	buf := make([]byte, 32)
	_, err = ptyAccess.Read(buf)
	if err == nil {
		t.Error("Read() should fail with nil PTY")
	}
}

func TestPTYAccess_GetSessionName(t *testing.T) {
	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", nil, buffer)

	sessionName := ptyAccess.GetSessionName()
	if sessionName != "test-session" {
		t.Errorf("GetSessionName() returned %q, expected %q", sessionName, "test-session")
	}
}
