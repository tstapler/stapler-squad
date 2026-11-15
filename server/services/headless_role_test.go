package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHeadlessRoleOf_should_MapEachPrefixToItsRole(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		uuid     string
		wantRole HeadlessRole
		wantOK   bool
	}{
		{"triage", "headless-triage-abc", HeadlessRoleTriage, true},
		{"re-review", "headless-re-review-abc", HeadlessRoleReReview, true},
		{"tmux-backed uuid", "7d6f0c1e-0000-0000-0000-000000000000", "", false},
		{"jules", "jules-sessions/x", "", false},
		{"prefix only", HeadlessTriageUUIDPrefix, HeadlessRoleTriage, true},
		{"empty", "", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			role, ok := HeadlessRoleOf(tc.uuid)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantRole, role)
			assert.Equal(t, tc.wantOK && tc.wantRole == HeadlessRoleTriage, IsHeadlessRole(tc.uuid, HeadlessRoleTriage))
		})
	}
}

func TestHeadlessRolePrefixes_should_BeUnambiguous(t *testing.T) {
	t.Parallel()
	for i, a := range headlessRolePrefixes {
		role, ok := HeadlessRoleOf(a.prefix + "x")
		assert.True(t, ok)
		assert.Equal(t, a.role, role)
		for j, b := range headlessRolePrefixes {
			if i != j {
				assert.False(t, strings.HasPrefix(a.prefix, b.prefix), "%q shadows %q", b.prefix, a.prefix)
			}
		}
	}
}
