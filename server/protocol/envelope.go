package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
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
	envelope := make([]byte, 5+len(data))
	envelope[0] = flags
	binary.BigEndian.PutUint32(envelope[1:5], uint32(len(data)))
	copy(envelope[5:], data)
	return envelope
}

// ReadEnvelope reads a single envelope from an io.Reader.
func ReadEnvelope(r io.Reader) (*Envelope, error) {
	// Read header (5 bytes)
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("failed to read envelope header: %w", err)
	}

	flags := header[0]
	length := binary.BigEndian.Uint32(header[1:5])

	// Read data
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("failed to read envelope data: %w", err)
	}

	return &Envelope{
		Flags:  flags,
		Length: length,
		Data:   data,
	}, nil
}

// WriteEnvelope writes an envelope to an io.Writer.
func WriteEnvelope(w io.Writer, flags byte, data []byte) error {
	envelope := CreateEnvelope(flags, data)
	if _, err := w.Write(envelope); err != nil {
		return fmt.Errorf("failed to write envelope: %w", err)
	}
	return nil
}

// IsEndStream checks if the envelope has the EndStream flag set.
func (e *Envelope) IsEndStream() bool {
	return (e.Flags & EndStreamFlag) == EndStreamFlag
}

// IsCompressed checks if the envelope has the Compressed flag set.
func (e *Envelope) IsCompressed() bool {
	return (e.Flags & CompressedFlag) == CompressedFlag
}
