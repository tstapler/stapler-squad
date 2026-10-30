package user

import (
	"testing"

	"example.com/session"
)

func TestRaw(t *testing.T) {
	session.StartSessionDriver(&session.Instance{}, "/tmp") // want `raw session.StartSessionDriver`
}

func TestAnnotated(t *testing.T) {
	//nolint:nosessiondrivertest asserts the refused-start path, nothing spawns
	session.StartSessionDriver(&session.Instance{}, "/tmp")
}

func TestBareNolint(t *testing.T) {
	//nolint:nosessiondrivertest
	session.StartSessionDriver(&session.Instance{}, "/tmp") // want `requires a reason`
}
