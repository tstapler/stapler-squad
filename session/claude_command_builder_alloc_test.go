package session

import "testing"

func TestIsValidUUID_NoAllocsForRegexCompile(t *testing.T) {
	const id = "550e8400-e29b-41d4-a716-446655440000"
	if !isValidUUID(id) || isValidUUID("nope") {
		t.Fatal("isValidUUID wrong result")
	}
	if allocs := testing.AllocsPerRun(100, func() { isValidUUID(id) }); allocs > 1 {
		t.Fatalf("allocs/op = %v, regex is being recompiled per call", allocs)
	}
}
