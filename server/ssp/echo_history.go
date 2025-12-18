package ssp

import (
	"sync"
	"time"
)

// EchoEntry represents a single input echo tracking entry.
type EchoEntry struct {
	ClientID          string
	EchoNum           uint64
	ClientTimestampMs int64
	ServerTimestampMs int64
	Data              []byte
	Acknowledged      bool
}

// EchoHistory tracks user input for predictive echo acknowledgment.
// Uses a ring buffer to efficiently track recent inputs and their ack status.
type EchoHistory struct {
	mu       sync.Mutex
	entries  []EchoEntry
	size     int
	writeIdx int

	// Per-client tracking of last acknowledged echo number
	lastAcked map[string]uint64

	// Per-client tracking of highest received echo number
	highestReceived map[string]uint64

	// Timeout for considering an input as processed
	ackTimeout time.Duration
}

// NewEchoHistory creates a new echo history with the given capacity.
func NewEchoHistory(size int) *EchoHistory {
	if size <= 0 {
		size = 1000 // Default
	}

	return &EchoHistory{
		entries:         make([]EchoEntry, size),
		size:            size,
		writeIdx:        0,
		lastAcked:       make(map[string]uint64),
		highestReceived: make(map[string]uint64),
		ackTimeout:      50 * time.Millisecond, // Mosh default
	}
}

// Record adds a new input entry for echo tracking.
func (h *EchoHistory) Record(clientID string, echoNum uint64, clientTimestampMs int64, data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Store in ring buffer
	h.entries[h.writeIdx] = EchoEntry{
		ClientID:          clientID,
		EchoNum:           echoNum,
		ClientTimestampMs: clientTimestampMs,
		ServerTimestampMs: time.Now().UnixMilli(),
		Data:              data,
		Acknowledged:      false,
	}

	h.writeIdx = (h.writeIdx + 1) % h.size

	// Track highest received for this client
	if echoNum > h.highestReceived[clientID] {
		h.highestReceived[clientID] = echoNum
	}
}

// MarkAcknowledged marks echo entries up to the given echoNum as acknowledged.
// Called when PTY output is received, indicating input has been processed.
func (h *EchoHistory) MarkAcknowledged(clientID string, upToEchoNum uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for i := range h.entries {
		if h.entries[i].ClientID == clientID && h.entries[i].EchoNum <= upToEchoNum {
			h.entries[i].Acknowledged = true
		}
	}

	if upToEchoNum > h.lastAcked[clientID] {
		h.lastAcked[clientID] = upToEchoNum
	}
}

// GetAckNum returns the highest echo number that should be acknowledged.
// Uses timeout-based heuristic: if input is older than ackTimeout, consider it processed.
func (h *EchoHistory) GetAckNum(clientID string) uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now().UnixMilli()
	timeoutMs := h.ackTimeout.Milliseconds()

	// Find the highest echo number older than timeout
	var highestReady uint64 = h.lastAcked[clientID]

	for i := range h.entries {
		entry := &h.entries[i]
		if entry.ClientID != clientID {
			continue
		}
		if entry.EchoNum == 0 {
			continue // Empty slot
		}

		// If entry is older than timeout, consider it processed
		age := now - entry.ServerTimestampMs
		if age >= timeoutMs && entry.EchoNum > highestReady {
			highestReady = entry.EchoNum
			entry.Acknowledged = true
		}
	}

	return highestReady
}

// GetPendingCount returns the number of unacknowledged entries for a client.
func (h *EchoHistory) GetPendingCount(clientID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	count := 0
	for i := range h.entries {
		if h.entries[i].ClientID == clientID && !h.entries[i].Acknowledged && h.entries[i].EchoNum > 0 {
			count++
		}
	}
	return count
}

// SetAckTimeout sets the timeout for considering input as processed.
func (h *EchoHistory) SetAckTimeout(timeout time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ackTimeout = timeout
}

// GetHighestReceived returns the highest echo number received from a client.
func (h *EchoHistory) GetHighestReceived(clientID string) uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.highestReceived[clientID]
}

// Clear removes all entries for a client (e.g., on disconnect).
func (h *EchoHistory) Clear(clientID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for i := range h.entries {
		if h.entries[i].ClientID == clientID {
			h.entries[i] = EchoEntry{}
		}
	}

	delete(h.lastAcked, clientID)
	delete(h.highestReceived, clientID)
}
