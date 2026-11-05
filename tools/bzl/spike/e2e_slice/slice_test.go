package e2e_slice

import (
	"os"
	"testing"
)

func TestSliceReadsInsideAndBinary(t *testing.T) {
	if _, err := os.ReadFile("inside.txt"); err != nil {
		t.Fatal(err)
	}
	if bin := os.Getenv("E2E_BIN"); bin != "" {
		if _, err := os.Stat(bin); err != nil {
			t.Fatal(err)
		}
	}
	_ = Marker
}
