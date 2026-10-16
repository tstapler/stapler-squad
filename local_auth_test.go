package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequireLoopbackListenAddr(t *testing.T) {
	for _, ok := range []string{"localhost:8543", "127.0.0.1:8543", "[::1]:8543", "localhost:0"} {
		assert.NoError(t, requireLoopbackListenAddr(ok), ok)
	}
	for _, bad := range []string{"0.0.0.0:8543", ":8543", "192.168.1.5:8543", "example.com:8543", "8543", ""} {
		assert.Error(t, requireLoopbackListenAddr(bad), bad)
	}
}
