package warren

import (
	"errors"
	"strings"
)

// MultiError collects multiple errors from phased startup or shutdown.
// It implements the error interface and supports errors.Is / errors.As unwrapping.
type MultiError struct {
	Errors []error
}

func (m *MultiError) Error() string {
	if len(m.Errors) == 1 {
		return m.Errors[0].Error()
	}
	msgs := make([]string, len(m.Errors))
	for i, e := range m.Errors {
		msgs[i] = "  - " + e.Error()
	}
	return "warren: multiple errors:\n" + strings.Join(msgs, "\n")
}

// Unwrap returns all contained errors for use with errors.Is / errors.As.
func (m *MultiError) Unwrap() []error {
	return m.Errors
}

// multiError returns nil if errs is empty, a plain error if there is exactly
// one, or a *MultiError otherwise. Call sites should use this instead of
// constructing *MultiError directly.
func multiError(errs []error) error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return &MultiError{Errors: errs}
	}
}

// IsMultiError reports whether err is or wraps a *MultiError.
func IsMultiError(err error) bool {
	var m *MultiError
	return errors.As(err, &m)
}
