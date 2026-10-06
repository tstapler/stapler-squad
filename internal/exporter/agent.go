package exporter

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/tstapler/stapler-squad/internal/history"
)

// AgentExporter handles exporting canonical events to Agent format.
type AgentExporter struct {
	writer io.Writer
	limit  int
}

// NewAgentExporter creates a new AgentExporter.
func NewAgentExporter(w io.Writer, limit int) *AgentExporter {
	if w == nil {
		w = os.Stdout
	}
	if limit <= 0 {
		limit = 100 // Default pagination/top-k limit
	}
	return &AgentExporter{writer: w, limit: limit}
}

// Export writes sanitized events to JSON, applying time-bounding and pagination limits.
func (e *AgentExporter) Export(events []*history.Event) error {
	// Apply time-bounding/pagination limits to avoid context overload
	if len(events) > e.limit {
		// Take the most recent 'limit' events
		events = events[len(events)-e.limit:]
	}

	// We assume inhibition rules are applied upstream, but we strictly enforce
	// that we only output events to the agent (or let caller handle it).
	// For now, we encode to JSON and write to stdout/writer.

	encoder := json.NewEncoder(e.writer)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(events); err != nil {
		return fmt.Errorf("failed to encode events for agent: %w", err)
	}

	return nil
}
