// Package nosessiondrivertest is a deliberate nosessiondrivertest violation; never import it.
package nosessiondrivertest

import (
	"testing"

	"github.com/tstapler/stapler-squad/session"
)

func TestViolation(t *testing.T) {
	session.StartSessionDriver(&session.Instance{}, "/tmp")
}
