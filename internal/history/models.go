package history

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Event represents a canonical history command event.
type Event struct {
	ID            string    `db:"id" json:"id"`                           // Unique identifier/hash
	Command       string    `db:"command" json:"command"`                 // The actual shell command
	Timestamp     time.Time `db:"timestamp" json:"timestamp"`             // When the command was executed
	Directory     string    `db:"directory" json:"directory"`             // Directory where the command was executed (if known)
	ExitCode      int       `db:"exit_code" json:"exit_code"`             // Exit code of the command (if known)
	ProgramSource string    `db:"program_source" json:"program_source"`   // e.g., "bash", "zsh"
	IsRedacted    bool      `db:"is_redacted" json:"is_redacted"`         // Whether the command has been sanitized
}

// GenerateID creates a unique composite key/hash for the event.
// For Bash without timestamps, it uses a fallback deduplication strategy
// relying on contiguous deduplication combined with content hashing.
func (e *Event) GenerateID(sequenceNumber int) {
	// Use sequenceNumber for disambiguating identical commands executed sequentially in Bash.
	// We hash the command, source, and either the timestamp or sequence number.
	data := fmt.Sprintf("%s:%s:%d:%d", e.ProgramSource, e.Command, e.Timestamp.UnixNano(), sequenceNumber)
	
	hash := sha256.Sum256([]byte(data))
	e.ID = hex.EncodeToString(hash[:])
}
