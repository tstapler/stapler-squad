// Package novartestseam is a deliberate novartestseam violation; never import it.
package novartestseam

import "testing"

var baseURL = "http://localhost:8543"

func TestViolation(t *testing.T) {
	baseURL = "http://127.0.0.1:0"
}
