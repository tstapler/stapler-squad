package session_test

import (
	"testing"

	"github.com/tstapler/stapler-squad/session"
)

func TestValidateWorkflowSlug(t *testing.T) {
	tests := []struct {
		name    string
		slug    string
		wantErr bool
	}{
		{name: "valid simple", slug: "my-workflow", wantErr: false},
		{name: "valid alphanumeric", slug: "workflow1", wantErr: false},
		{name: "valid with numbers", slug: "daily-standup-v2", wantErr: false},
		{name: "valid min length", slug: "ab", wantErr: false},
		{name: "valid max length", slug: "abcdefghijklmnopqrstuvwxyz01234567890123456789012345678901234", wantErr: false},
		{name: "valid exactly 64 chars", slug: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", wantErr: false},
		{name: "invalid 65 chars", slug: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", wantErr: true},
		// Invalid cases
		{name: "empty", slug: "", wantErr: true},
		{name: "too short single char", slug: "a", wantErr: true},
		{name: "uppercase", slug: "My-Workflow", wantErr: true},
		{name: "leading hyphen", slug: "-workflow", wantErr: true},
		{name: "trailing hyphen", slug: "workflow-", wantErr: true},
		{name: "consecutive hyphens", slug: "work--flow", wantErr: true},
		{name: "spaces", slug: "my workflow", wantErr: true},
		{name: "underscore", slug: "my_workflow", wantErr: true},
		{name: "too long", slug: "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz012345", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := session.ValidateWorkflowSlug(tt.slug)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWorkflowSlug(%q) error = %v, wantErr %v", tt.slug, err, tt.wantErr)
			}
		})
	}
}
