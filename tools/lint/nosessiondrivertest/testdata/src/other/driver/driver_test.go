package driver

import (
	"testing"

	real "example.com/driver"
)

func TestSamePackageNameDifferentPath(t *testing.T) {
	real.StartSessionDriver(nil, "") // want `raw driver.StartSessionDriver`
}
