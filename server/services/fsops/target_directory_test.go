package fsops

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateTargetDirectory_should_RequireAbsoluteCleanPaths(t *testing.T) {
	t.Parallel()
	const notAbs = "target_directory must be an absolute path"
	const traversal = "target_directory contains path traversal components"
	cases := []struct {
		dir  string
		want string
	}{
		{"/srv/work", ""},
		{"/", ""},
		{"", notAbs},
		{"work", notAbs},
		{"./work", notAbs},
		{"../work", notAbs},
		{"~/work", notAbs},
		{"/srv/../etc", traversal},
		{"/srv/./work", traversal},
		{"/srv/work/", traversal},
		{"//srv/work", traversal},
		{"/srv//work", traversal},
		{"/srv/work/..", traversal},
	}
	for _, c := range cases {
		err := ValidateTargetDirectory(c.dir)
		if c.want == "" {
			assert.NoError(t, err, c.dir)
			continue
		}
		assert.EqualError(t, err, c.want, c.dir)
	}
}
