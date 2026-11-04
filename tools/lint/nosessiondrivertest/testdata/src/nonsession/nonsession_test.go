package nonsession

import "testing"

type runner struct{}

func (runner) StartSessionDriver() {}

func StartSessionDriver() {}

func TestUnrelated(t *testing.T) {
	runner{}.StartSessionDriver()
	StartSessionDriver()
}
