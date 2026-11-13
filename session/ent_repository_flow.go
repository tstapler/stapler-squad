package session

import (
	"fmt"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
)

// EntFlowReader implements the flow analytics source interfaces (events,
// verdicts, stuck rows, items) over the repository's ent client (ADR-004).
//
// Every query is a plain single-statement SELECT, so dbsplit routes it to the
// query_only reader pool (label "ent-read") and never holds the writer's
// connection. It opens no database handle of its own.
type EntFlowReader struct {
	client *ent.Client
	engine domain.WorkflowEngine // source of Stages; nil means the default engine
}

// WithWorkflowEngine returns a copy of r whose Stages come from engine.
func (r *EntFlowReader) WithWorkflowEngine(engine domain.WorkflowEngine) *EntFlowReader {
	cp := *r
	cp.engine = engine
	return &cp
}

// NewEntFlowReader returns a reader over client, normally repo.GetEntClient().
func NewEntFlowReader(client *ent.Client) *EntFlowReader {
	return &EntFlowReader{client: client}
}

// FlowReadError wraps a failed flow read with the operation name so callers
// can name the failed section; it unwraps to the underlying cause.
type FlowReadError struct {
	Op  string
	Err error
}

func (e *FlowReadError) Error() string { return fmt.Sprintf("flow read %s: %v", e.Op, e.Err) }

func (e *FlowReadError) Unwrap() error { return e.Err }

func flowReadErr(op string, err error) error {
	if err == nil {
		return nil
	}
	return &FlowReadError{Op: op, Err: err}
}
