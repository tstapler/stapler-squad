package exporter

import (
	"bytes"
	"fmt"
	"os"

	"github.com/tstapler/stapler-squad/internal/history"
)

// BashExporter handles exporting canonical events to a Bash history file.
type BashExporter struct {
	filepath string
}

// NewBashExporter creates a new BashExporter.
func NewBashExporter(filepath string) *BashExporter {
	return &BashExporter{filepath: filepath}
}

// Export appends canonical events to the target Bash history file.
// It relies on standard POSIX O_APPEND (atomic for writes under PIPE_BUF).
// Users should configure HISTAPPEND in their shells.
func (e *BashExporter) Export(events []*history.Event) error {
	file, err := os.OpenFile(e.filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open bash history file: %w", err)
	}
	defer file.Close()

	var buf bytes.Buffer
	for _, ev := range events {
		// Target Bash format with timestamps:
		// #1600000000
		// command
		ts := ev.Timestamp.Unix()
		
		// For multiline bash commands, typically just literal newlines
		cmd := ev.Command
		
		buf.WriteString(fmt.Sprintf("#%d\n%s\n", ts, cmd))
	}

	_, err = file.Write(buf.Bytes())
	return err
}
