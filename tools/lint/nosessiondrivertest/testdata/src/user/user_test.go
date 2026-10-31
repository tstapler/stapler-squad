package user

import (
	"testing"

	"example.com/session"
	s "example.com/session"
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

func TestTrailingCommentDoesNotCoverNextLine(t *testing.T) {
	session.StartSessionDriver(&session.Instance{}, "/tmp") //nolint:nosessiondrivertest reason one
	session.StartSessionDriver(&session.Instance{}, "/tmp") // want `raw session.StartSessionDriver`
}

func TestMultiLineCallWithNolintAbove(t *testing.T) {
	//nolint:nosessiondrivertest asserts the refused-start path
	session.StartSessionDriver(
		&session.Instance{},
		"/tmp",
	)
}

func TestAliased(t *testing.T) {
	s.StartSessionDriver(&s.Instance{}, "/tmp") // want `raw session.StartSessionDriver`
}

func TestMethodValue(t *testing.T) {
	f := session.StartSessionDriver // want `raw session.StartSessionDriver`
	f(nil, "")
}

func TestGoStatement(t *testing.T) {
	go session.StartSessionDriver(nil, "") // want `raw session.StartSessionDriver`
}

func TestInRunClosure(t *testing.T) {
	t.Run("sub", func(t *testing.T) {
		session.StartSessionDriver(nil, "") // want `raw session.StartSessionDriver`
	})
}
