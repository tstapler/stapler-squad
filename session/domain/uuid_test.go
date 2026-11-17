package domain

import "testing"

func TestIsValidUUID_NoAllocsForRegexCompile(t *testing.T) {
	const id = "550e8400-e29b-41d4-a716-446655440000"
	if !IsValidUUID(id) || IsValidUUID("nope") {
		t.Fatal("IsValidUUID wrong result")
	}
	if allocs := testing.AllocsPerRun(100, func() { IsValidUUID(id) }); allocs > 1 {
		t.Fatalf("allocs/op = %v, regex is being recompiled per call", allocs)
	}
}
