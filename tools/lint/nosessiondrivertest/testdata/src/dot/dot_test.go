package dot

import (
	"testing"

	. "example.com/session"
)

func TestDotImport(t *testing.T) {
	StartSessionDriver(nil, "") // want `raw session.StartSessionDriver`
}
