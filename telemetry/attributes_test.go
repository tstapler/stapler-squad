package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSubprocessCommandAttr_should_ReturnAttributeWithCommandNameValue_When_GivenNonEmptyCommand
// covers the typed-attribute-constructor convention (see the Attr<Concept>
// constant + <Concept>Attr(...) pattern documented in this file) for the
// subprocess command name attribute added for the go-auto-instrumentation
// project's Phase 5 subprocess hook.
func TestSubprocessCommandAttr_should_ReturnAttributeWithCommandNameValue_When_GivenNonEmptyCommand(t *testing.T) {
	attr := SubprocessCommandAttr("git")

	assert.Equal(t, AttrSubprocessCommand, string(attr.Key))
	assert.Equal(t, "git", attr.Value.AsString())
}

func TestSubprocessCommandAttr_should_ReturnEmptyValueAttribute_When_GivenEmptyCommand(t *testing.T) {
	attr := SubprocessCommandAttr("")

	assert.Equal(t, AttrSubprocessCommand, string(attr.Key))
	assert.Equal(t, "", attr.Value.AsString())
}

func TestSubprocessArgCountAttr_should_ReturnIntAttribute_When_GivenArgCount(t *testing.T) {
	tests := []struct {
		name  string
		count int
	}{
		{name: "zero args", count: 0},
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
