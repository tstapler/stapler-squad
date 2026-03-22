package ssp

import (
	"time"

	"github.com/tstapler/stapler-squad/session"
)

// ClientState tracks the SSP state for a single connected client.
// Each client has its own view of the terminal state for accurate diffing.
type ClientState struct {
	// ClientID is the unique identifier for this client
	ClientID string

	// Capabilities negotiated during connection
	Capabilities *ClientCapabilities

	// LastFramebuffer is the client's last known terminal state
	// Used as the base for generating diffs
	LastFramebuffer *session.TerminalState

	// LastSequence is the sequence number of the last state sent to client
	LastSequence uint64

	// LastEchoAckNum is the last echo acknowledgment sent to client
	LastEchoAckNum uint64

	// RTT estimation using TCP-style SRTT calculation
	SRTT time.Duration // Smoothed RTT

	// Connected timestamp
	ConnectedAt time.Time

	// Statistics
	Stats ClientStats
}

// ClientCapabilities represents the SSP features a client supports.
type ClientCapabilities struct {
	// SupportsPredictiveEcho indicates the client can handle predictive echo
	SupportsPredictiveEcho bool

	// SupportsDiffUpdates indicates the client can process diff messages
	SupportsDiffUpdates bool

	// CompressionAlgorithms supported by the client
	CompressionAlgorithms []string

	// ProtocolVersion is the SSP protocol version supported
	ProtocolVersion uint32

	// MaxDiffSize is the maximum diff size the client can handle
	MaxDiffSize uint32

	// PreferredFrameIntervalMs is the client's preferred update rate
	PreferredFrameIntervalMs uint32
}

// ClientStats tracks statistics for a client connection.
type ClientStats struct {
	// BytesSent is the total bytes sent to this client
	BytesSent uint64

	// DiffsSent is the number of diff updates sent
	DiffsSent uint64

	// FullRedraws is the number of full redraws sent
	FullRedraws uint64

	// DroppedFrames is the number of frames dropped due to throttling
	DroppedFrames uint64

	// EchoAcks is the number of echo acknowledgments sent
	EchoAcks uint64

	// ResyncRequests is the number of resync requests from client
	ResyncRequests uint64
}

// NewClientState creates a new client state with the given capabilities.
func NewClientState(clientID string, capabilities *ClientCapabilities) *ClientState {
	if capabilities == nil {
		capabilities = &ClientCapabilities{
			SupportsPredictiveEcho: false,
			SupportsDiffUpdates:    true,
			ProtocolVersion:        1,
		}
	}

	return &ClientState{
		ClientID:        clientID,
		Capabilities:    capabilities,
		LastFramebuffer: nil,
		LastSequence:    0,
		LastEchoAckNum:  0,
		SRTT:            100 * time.Millisecond, // Initial estimate
		ConnectedAt:     time.Now(),
	}
}

// UpdateRTT updates the smoothed RTT using TCP-style calculation.
// SRTT = (1 - alpha) * SRTT + alpha * sample
// Using alpha = 0.125 (TCP default)
func (cs *ClientState) UpdateRTT(sample time.Duration) {
	const alpha = 0.125
	cs.SRTT = time.Duration(float64(cs.SRTT)*(1-alpha) + float64(sample)*alpha)
}

// GetMinFrameInterval returns the recommended minimum frame interval
// based on RTT and client preferences.
func (cs *ClientState) GetMinFrameInterval() time.Duration {
	// Use 2x RTT as minimum interval to avoid overwhelming slow connections
	rttBased := cs.SRTT * 2

	// Respect client preference if set
	if cs.Capabilities != nil && cs.Capabilities.PreferredFrameIntervalMs > 0 {
		preferred := time.Duration(cs.Capabilities.PreferredFrameIntervalMs) * time.Millisecond
		if preferred > rttBased {
			return preferred
		}
	}

	// Minimum of 16ms (60fps) even on fast connections
	if rttBased < 16*time.Millisecond {
		return 16 * time.Millisecond
	}

	return rttBased
}

// CanHandleDiff returns true if the client supports diff updates.
func (cs *ClientState) CanHandleDiff() bool {
	return cs.Capabilities != nil && cs.Capabilities.SupportsDiffUpdates
}

// CanHandlePredictiveEcho returns true if the client supports predictive echo.
func (cs *ClientState) CanHandlePredictiveEcho() bool {
	return cs.Capabilities != nil && cs.Capabilities.SupportsPredictiveEcho
}

// RecordDiffSent records statistics for a sent diff.
func (cs *ClientState) RecordDiffSent(size int, fullRedraw bool) {
	cs.Stats.BytesSent += uint64(size)
	if fullRedraw {
		cs.Stats.FullRedraws++
	} else {
		cs.Stats.DiffsSent++
	}
}

// RecordDroppedFrame records a dropped frame due to throttling.
func (cs *ClientState) RecordDroppedFrame() {
	cs.Stats.DroppedFrames++
}

// RecordEchoAck records a sent echo acknowledgment.
func (cs *ClientState) RecordEchoAck() {
	cs.Stats.EchoAcks++
}

// RecordResync records a resync request from client.
func (cs *ClientState) RecordResync() {
	cs.Stats.ResyncRequests++
}
