package session

import (
	"testing"

	real "example.com/session"
)

func TestSamePackageNameDifferentPath(t *testing.T) {
	real.StartSessionDriver(nil, "") // want `raw session.StartSessionDriver`
}
