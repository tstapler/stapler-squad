// Package nosessiondrivertest is a deliberate nosessiondrivertest violation; never import it.
package nosessiondrivertest

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/driver"
)

func TestViolation(t *testing.T) {
	driver.StartSessionDriver(nil, "/tmp")
}
