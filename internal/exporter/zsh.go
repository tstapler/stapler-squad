package exporter

import (
	"bytes"
	"fmt"
	"os"

	"github.com/tstapler/stapler-squad/internal/history"
)

// ZshExporter handles exporting canonical events to a Zsh history file.
type ZshExporter struct {
	filepath string
}

// NewZshExporter creates a new ZshExporter.
func NewZshExporter(filepath string) *ZshExporter {
	return &ZshExporter{filepath: filepath}
}

// metafy simply escapes characters Zsh requires, mainly replacing \n with \\\n.
func (e *ZshExporter) metafy(cmd string) string {
	// Simple escaping: add backslash before newlines
	return string(bytes.ReplaceAll([]byte(cmd), []byte("\n"), []byte("\\\n")))
}

// Export appends canonical events to the target Zsh history file.
// It relies on standard POSIX O_APPEND (atomic for writes under PIPE_BUF).
// Users should configure HISTAPPEND/share_history in their shells.
func (e *ZshExporter) Export(events []*history.Event) error {
	file, err := os.OpenFile(e.filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open zsh history file: %w", err)
	}
	defer file.Close()

	var buf bytes.Buffer
	for _, ev := range events {
		// Target Zsh extended format: : 1600000000:0;command
		ts := ev.Timestamp.Unix()
		cmd := string(e.metafy(ev.Command))
		
		buf.WriteString(fmt.Sprintf(": %d:0;%s\n", ts, cmd))
	}

	_, err = file.Write(buf.Bytes())
	return err
}
