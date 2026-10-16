package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSubprocessCommandAttr covers the typed-attribute-constructor
// convention (see the Attr<Concept> constant + <Concept>Attr(...) pattern
// documented in this file) for the subprocess command name attribute added
// for the go-auto-instrumentation project's Phase 5 subprocess hook.
func TestSubprocessCommandAttr(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{name: "non-empty command", cmd: "git"},
		{name: "empty command", cmd: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := SubprocessCommandAttr(tt.cmd)

			assert.Equal(t, AttrSubprocessCommand, string(attr.Key))
			assert.Equal(t, tt.cmd, attr.Value.AsString())
		})
	}
}

func TestSubprocessArgCountAttr_should_ReturnIntAttribute_When_GivenArgCount(t *testing.T) {
	tests := []struct {
		name  string
		count int
	}{
		{name: "zero args", count: 0},
		{name: "one arg", count: 1},
		{name: "several args", count: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := SubprocessArgCountAttr(tt.count)

			assert.Equal(t, AttrSubprocessArgCount, string(attr.Key))
			assert.Equal(t, int64(tt.count), attr.Value.AsInt64())
		})
	}
}
