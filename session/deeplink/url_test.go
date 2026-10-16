package deeplink

import (
	"errors"
	"testing"
)

func TestParseDeepLink_should_ExtractHostnameTypeVersionAndID_When_URLWellFormed(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantLink DeepLink
	}{
		{
			name: "backlog item on plain hostname",
			raw:  "ssq://myhost/backlog/v1/bl_01J0000000000000000000",
			wantLink: DeepLink{
				Hostname: "myhost",
				ItemType: "backlog",
				Version:  "v1",
				ID:       "bl_01J0000000000000000000",
			},
		},
		{
			name: "hostname with port",
			raw:  "ssq://myhost.example.com:8543/backlog/v1/bl_01J0000000000000000000",
			wantLink: DeepLink{
				Hostname: "myhost.example.com:8543",
				ItemType: "backlog",
				Version:  "v1",
				ID:       "bl_01J0000000000000000000",
			},
		},
		{
			name: "legacy uuid id",
			raw:  "ssq://myhost/backlog/v1/550e8400-e29b-41d4-a716-446655440000",
			wantLink: DeepLink{
				Hostname: "myhost",
				ItemType: "backlog",
				Version:  "v1",
				ID:       "550e8400-e29b-41d4-a716-446655440000",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDeepLink(tt.raw)
			if err != nil {
				t.Fatalf("ParseDeepLink(%q) returned unexpected error: %v", tt.raw, err)
			}
			if got != tt.wantLink {
				t.Errorf("ParseDeepLink(%q) = %+v, want %+v", tt.raw, got, tt.wantLink)
			}
		})
	}
}

func TestParseDeepLink_should_ReturnErrUnsupportedVersion_When_VersionNotRecognizedByThisBinary(t *testing.T) {
	malformedTests := []struct {
		name string
		raw  string
	}{
		{name: "truncated URL", raw: "ssq://"},
		{name: "missing segments", raw: "ssq://myhost/backlog/v1"},
		{name: "extra segments", raw: "ssq://myhost/backlog/v1/id123/extra"},
		{name: "wrong scheme", raw: "https://myhost/backlog/v1/bl_01J0000000000000000000"},
		{name: "missing hostname", raw: "ssq:///backlog/v1/bl_01J0000000000000000000"},
		{name: "too few segments (empty leading segment collapses)", raw: "ssq://myhost//v1/bl_01J0000000000000000000"},
		{name: "too few segments (trailing slash collapses)", raw: "ssq://myhost/backlog/v1/"},
		{name: "empty middle segment with segment count still 3", raw: "ssq://myhost/backlog//bl_01J0000000000000000000"},
		{name: "not a URL at all", raw: "::::not a url::::"},
		{name: "invalid URL escape", raw: "ssq://myhost/backlog/v1/%zz"},
	}
	for _, tt := range malformedTests {
		t.Run("malformed/"+tt.name, func(t *testing.T) {
			_, err := ParseDeepLink(tt.raw)
			if !errors.Is(err, ErrMalformed) {
				t.Errorf("ParseDeepLink(%q) error = %v, want errors.Is(err, ErrMalformed)", tt.raw, err)
			}
			if errors.Is(err, ErrUnsupportedVersion) {
				t.Errorf("ParseDeepLink(%q) unexpectedly also matched ErrUnsupportedVersion", tt.raw)
			}
		})
	}

	versionTests := []struct {
		name string
		raw  string
	}{
		{name: "future version", raw: "ssq://myhost/backlog/v2/bl_01J0000000000000000000"},
		{name: "non-numeric version", raw: "ssq://myhost/backlog/vNext/bl_01J0000000000000000000"},
	}
	for _, tt := range versionTests {
		t.Run("unsupported-version/"+tt.name, func(t *testing.T) {
			got, err := ParseDeepLink(tt.raw)
			if !errors.Is(err, ErrUnsupportedVersion) {
				t.Fatalf("ParseDeepLink(%q) error = %v, want errors.Is(err, ErrUnsupportedVersion)", tt.raw, err)
			}
			if got != (DeepLink{}) {
				t.Errorf("ParseDeepLink(%q) returned non-zero DeepLink %+v on unsupported version, want zero value", tt.raw, got)
			}
		})
	}
}
