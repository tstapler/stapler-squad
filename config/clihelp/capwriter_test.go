package clihelp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCapWriter_should_KeepFirstNAndFlagOnce_When_WritesExceedCap(t *testing.T) {
	var overflows int
	w := &capWriter{max: 5, onOverflow: func() { overflows++ }}

	n, err := w.Write([]byte("abc"))
	assert.Equal(t, 3, n)
	assert.NoError(t, err)
	assert.False(t, w.truncated)

	n, err = w.Write([]byte("defgh"))
	assert.Equal(t, 5, n, "always reports the full length so the child never blocks")
	assert.NoError(t, err)
	n, _ = w.Write([]byte("ijk"))
	assert.Equal(t, 3, n)

	assert.Equal(t, "abcde", w.buf.String())
	assert.True(t, w.truncated)
	assert.Equal(t, 1, overflows)
}

func TestCapWriter_should_NotTruncate_When_OutputExactlyFillsCap(t *testing.T) {
	w := &capWriter{max: 3}
	_, _ = w.Write([]byte("abc"))
	assert.False(t, w.truncated)
}
