package user

import (
	"testing"

	"example.com/driver"
	s "example.com/driver"
)

func TestRaw(t *testing.T) {
	driver.StartSessionDriver(&driver.Instance{}, "/tmp") // want `raw driver.StartSessionDriver`
}

func TestAnnotated(t *testing.T) {
	//nolint:nosessiondrivertest asserts the refused-start path, nothing spawns
	driver.StartSessionDriver(&driver.Instance{}, "/tmp")
}

func TestBareNolint(t *testing.T) {
	//nolint:nosessiondrivertest
	driver.StartSessionDriver(&driver.Instance{}, "/tmp") // want `requires a reason`
}

func TestTrailingCommentDoesNotCoverNextLine(t *testing.T) {
	driver.StartSessionDriver(&driver.Instance{}, "/tmp") //nolint:nosessiondrivertest reason one
	driver.StartSessionDriver(&driver.Instance{}, "/tmp") // want `raw driver.StartSessionDriver`
}

func TestMultiLineCallWithNolintAbove(t *testing.T) {
	//nolint:nosessiondrivertest asserts the refused-start path
	driver.StartSessionDriver(
		&driver.Instance{},
		"/tmp",
	)
}

func TestAliased(t *testing.T) {
	s.StartSessionDriver(&s.Instance{}, "/tmp") // want `raw driver.StartSessionDriver`
}

func TestMethodValue(t *testing.T) {
	f := driver.StartSessionDriver // want `raw driver.StartSessionDriver`
	f(nil, "")
}

func TestGoStatement(t *testing.T) {
	go driver.StartSessionDriver(nil, "") // want `raw driver.StartSessionDriver`
}

func TestInRunClosure(t *testing.T) {
	t.Run("sub", func(t *testing.T) {
		driver.StartSessionDriver(nil, "") // want `raw driver.StartSessionDriver`
	})
}
