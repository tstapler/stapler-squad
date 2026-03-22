// Package mux provides PTY multiplexing functionality for external Claude sessions.
// It enables bidirectional terminal streaming from external processes (like Claude Code
// running in IntelliJ) to stapler-squad for monitoring and interaction.
package mux

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// MessageType represents the type of message in the mux protocol.
type MessageType byte

const (
	// MessageTypeOutput is terminal output from the PTY (claude -> clients)
	MessageTypeOutput MessageType = 0x01
	// MessageTypeInput is terminal input to the PTY (clients -> claude)
	MessageTypeInput MessageType = 0x02
	// MessageTypeResize is a terminal resize event (SIGWINCH)
	MessageTypeResize MessageType = 0x03
	// MessageTypeMetadata is session metadata (command, pid, cwd, env)
	MessageTypeMetadata MessageType = 0x04
	// MessageTypePing is a keepalive ping
	MessageTypePing MessageType = 0x05
	// MessageTypePong is a keepalive pong response
	MessageTypePong MessageType = 0x06
	// MessageTypeClose signals graceful session close
	MessageTypeClose MessageType = 0x07
	// MessageTypeSnapshot requests a clean screen snapshot (capture-pane)
	MessageTypeSnapshot MessageType = 0x08
	// MessageTypeSnapshotReply contains the clean screen snapshot
	MessageTypeSnapshotReply MessageType = 0x09
)

// Message represents a single message in the mux protocol.
// Wire format: [1 byte: type] [4 bytes: length (big-endian)] [N bytes: data]
type Message struct {
	Type MessageType
	Data []byte
}

// ResizeData represents terminal resize dimensions.
type ResizeData struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// SessionMetadata contains information about the multiplexed session.
type SessionMetadata struct {
	Command     string            `json:"command"`      // The command being run (e.g., "claude")
	Args        []string          `json:"args"`         // Command arguments
	PID         int               `json:"pid"`          // Process ID of the child
	Cwd         string            `json:"cwd"`          // Current working directory
	Env         map[string]string `json:"env"`          // Selected environment variables
	SocketPath  string            `json:"socket_path"`  // Path to the Unix socket
	StartTime   int64             `json:"start_time"`   // Unix timestamp when session started
	TmuxSession string            `json:"tmux_session"` // Tmux session name (for stapler-squad adoption)
}

// EncodeMessage encodes a message to wire format.
func EncodeMessage(msg *Message) ([]byte, error) {
	if len(msg.Data) > 0xFFFFFFFF {
		return nil, fmt.Errorf("message data too large: %d bytes", len(msg.Data))
	}

	buf := make([]byte, 5+len(msg.Data))
	buf[0] = byte(msg.Type)
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(msg.Data)))
	copy(buf[5:], msg.Data)

	return buf, nil
}

// DecodeMessage reads and decodes a message from a reader.
func DecodeMessage(r io.Reader) (*Message, error) {
	// Read header: 1 byte type + 4 bytes length
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("failed to read message header: %w", err)
	}

	msgType := MessageType(header[0])
	length := binary.BigEndian.Uint32(header[1:5])

	// Sanity check on length to prevent memory exhaustion
	const maxMessageSize = 16 * 1024 * 1024 // 16 MB max
	if length > maxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes (max %d)", length, maxMessageSize)
	}

	// Read data
	data := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, fmt.Errorf("failed to read message data: %w", err)
		}
	}

	return &Message{
		Type: msgType,
		Data: data,
	}, nil
}

// WriteMessage writes an encoded message to a writer.
func WriteMessage(w io.Writer, msg *Message) error {
	encoded, err := EncodeMessage(msg)
	if err != nil {
		return err
	}

	_, err = w.Write(encoded)
	return err
}

// NewOutputMessage creates an output message from terminal data.
func NewOutputMessage(data []byte) *Message {
	return &Message{
		Type: MessageTypeOutput,
		Data: data,
	}
}

// NewInputMessage creates an input message to send to the PTY.
func NewInputMessage(data []byte) *Message {
	return &Message{
		Type: MessageTypeInput,
		Data: data,
	}
}

// NewResizeMessage creates a resize message with terminal dimensions.
func NewResizeMessage(cols, rows uint16) *Message {
	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], cols)
	binary.BigEndian.PutUint16(data[2:4], rows)
	return &Message{
		Type: MessageTypeResize,
		Data: data,
	}
}

// ParseResizeMessage extracts dimensions from a resize message.
func ParseResizeMessage(msg *Message) (*ResizeData, error) {
	if msg.Type != MessageTypeResize {
		return nil, fmt.Errorf("not a resize message: type %d", msg.Type)
	}
	if len(msg.Data) != 4 {
		return nil, fmt.Errorf("invalid resize data length: %d", len(msg.Data))
	}
	return &ResizeData{
		Cols: binary.BigEndian.Uint16(msg.Data[0:2]),
		Rows: binary.BigEndian.Uint16(msg.Data[2:4]),
	}, nil
}

// NewMetadataMessage creates a metadata message with session information.
func NewMetadataMessage(meta *SessionMetadata) (*Message, error) {
	data, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}
	return &Message{
		Type: MessageTypeMetadata,
		Data: data,
	}, nil
}

// ParseMetadataMessage extracts session metadata from a message.
func ParseMetadataMessage(msg *Message) (*SessionMetadata, error) {
	if msg.Type != MessageTypeMetadata {
		return nil, fmt.Errorf("not a metadata message: type %d", msg.Type)
	}
	var meta SessionMetadata
	if err := json.Unmarshal(msg.Data, &meta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}
	return &meta, nil
}

// NewPingMessage creates a ping keepalive message.
func NewPingMessage() *Message {
	return &Message{
		Type: MessageTypePing,
		Data: nil,
	}
}

// NewPongMessage creates a pong keepalive response.
func NewPongMessage() *Message {
	return &Message{
		Type: MessageTypePong,
		Data: nil,
	}
}

// NewCloseMessage creates a close message to signal session end.
func NewCloseMessage() *Message {
	return &Message{
		Type: MessageTypeClose,
		Data: nil,
	}
}

// NewSnapshotRequestMessage creates a snapshot request message.
// This requests a clean screen capture (tmux capture-pane) from the multiplexer.
func NewSnapshotRequestMessage() *Message {
	return &Message{
		Type: MessageTypeSnapshot,
		Data: nil,
	}
}

// NewSnapshotReplyMessage creates a snapshot reply with the captured content.
func NewSnapshotReplyMessage(content []byte) *Message {
	return &Message{
		Type: MessageTypeSnapshotReply,
		Data: content,
	}
}
