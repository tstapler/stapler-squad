package protocol

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Envelope represents a ConnectRPC envelope message with flags and data.
// Format: [1 byte flags][4 bytes length][N bytes data]
type Envelope struct {
	Flags  byte
	Length uint32
	Data   []byte
}

// Envelope protocol flags
const (
	CompressedFlag byte = 0x01 // Message is compressed
	EndStreamFlag  byte = 0x02 // Final message in stream
)

// ParseEnvelope reads an envelope from the provided data.
// Returns the parsed envelope and any remaining unconsumed data.
func ParseEnvelope(data []byte) (*Envelope, []byte, error) {
	if len(data) < 5 {
		return nil, data, fmt.Errorf("envelope too short: need at least 5 bytes, got %d", len(data))
	}

	flags := data[0]
	length := binary.BigEndian.Uint32(data[1:5])

	if len(data) < int(5+length) {
		return nil, data, fmt.Errorf("incomplete envelope: need %d bytes, got %d", 5+length, len(data))
	}

	envelope := &Envelope{
		Flags:  flags,
		Length: length,
		Data:   data[5 : 5+length],
	}

	remaining := data[5+length:]
	return envelope, remaining, nil
}

// CreateEnvelope creates an envelope with the given flags and data.
func CreateEnvelope(flags byte, data []byte) []byte {
	if len(data) > math.MaxUint32 {
		// In practice unreachable: no PTY output chunk, RPC response body, or
		// session snapshot this server produces approaches 4 GiB. Silently
		// truncating the uint32 length header while writing all of data
		// would desync the receiver's framing (Length would no longer match
		// Data), so failing loudly beats corrupting the wire protocol.
		panic(fmt.Sprintf("protocol: envelope data too large to encode: %d bytes", len(data)))
	}

	envelope := make([]byte, 5+len(data))
	envelope[0] = flags
	// #nosec G115 -- bounds-checked above: the len(data) > math.MaxUint32 guard guarantees this fits in uint32
	binary.BigEndian.PutUint32(envelope[1:5], uint32(len(data)))
	copy(envelope[5:], data)
	return envelope
}

// IsEndStream checks if the envelope has the EndStream flag set.
func (e *Envelope) IsEndStream() bool {
	return (e.Flags & EndStreamFlag) == EndStreamFlag
}

// IsCompressed checks if the envelope has the Compressed flag set.
func (e *Envelope) IsCompressed() bool {
	return (e.Flags & CompressedFlag) == CompressedFlag
}
