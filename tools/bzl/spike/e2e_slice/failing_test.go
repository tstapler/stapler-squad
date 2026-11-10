//go:build s14_failing

package e2e_slice

import "testing"

func TestAlwaysFails(t *testing.T) { t.Fatal("seeded failure (S-c)") }
