package cli

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ssh"
)

// An SSH runner's non-zero exit is an *ssh.ExitError, which has ExitStatus but no ExitCode.
var _ exitStatuser = (*ssh.ExitError)(nil)

type statusErr struct{ n int }

func (e statusErr) Error() string   { return "exit" }
func (e statusErr) ExitStatus() int { return e.n }

type codeErr struct{ n int }

func (e codeErr) Error() string { return "exit" }
func (e codeErr) ExitCode() int { return e.n }

func TestExitCode(t *testing.T) {
	assert.Equal(t, 128, exitCode(fmt.Errorf("wrapped: %w", statusErr{128})))
	assert.Equal(t, 1, exitCode(fmt.Errorf("wrapped: %w", codeErr{1})))
	assert.Equal(t, -1, exitCode(fmt.Errorf("dial failed")))
}
