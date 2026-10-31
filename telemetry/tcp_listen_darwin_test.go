//go:build darwin

package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadSomaxconn_should_ReturnKernelCeiling(t *testing.T) {
	v, ok := readSomaxconn()
	assert.True(t, ok)
	assert.Positive(t, v)
}
