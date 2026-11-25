package domain

import "testing"

func TestIsValidUUID_Table(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		uuid string
		want bool
	}{
		{"valid UUID v4", "550e8400-e29b-41d4-a716-446655440000", true},
		{"uppercase", "550E8400-E29B-41D4-A716-446655440000", true},
		{"missing section", "550e8400-e29b-41d4-446655440000", false},
		{"wrong format", "not-a-uuid", false},
		{"too short", "550e8400", false},
		{"no hyphens", "550e8400e29b41d4a716446655440000", false},
		{"empty string", "", false},
		{"non-hex character", "550e8400-e29b-41d4-a716-44665544000g", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsValidUUID(tt.uuid); got != tt.want {
				t.Errorf("IsValidUUID(%q) = %v, want %v", tt.uuid, got, tt.want)
			}
		})
	}
}

func TestIsValidUUID_NoAllocsForRegexCompile(t *testing.T) {
	const id = "550e8400-e29b-41d4-a716-446655440000"
	if !IsValidUUID(id) || IsValidUUID("nope") {
		t.Fatal("IsValidUUID wrong result")
	}
	if allocs := testing.AllocsPerRun(100, func() { IsValidUUID(id) }); allocs > 1 {
		t.Fatalf("allocs/op = %v, regex is being recompiled per call", allocs)
	}
}
