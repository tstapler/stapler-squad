package session

import (
	"errors"
)

// ErrZeroPRRef is returned by every PR repository entry point handed a PRRef{}.
var ErrZeroPRRef = errors.New("session: zero PRRef")
