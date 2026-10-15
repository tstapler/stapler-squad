package server

import (
	"testing"

	"github.com/tstapler/stapler-squad/pkg/buildinfo"
)

func TestIsSourceBuild(t *testing.T) {
	origV, origB, origC := buildinfo.Version, buildinfo.Branch, buildinfo.Commit
	t.Cleanup(func() { buildinfo.Version, buildinfo.Branch, buildinfo.Commit = origV, origB, origC })

	cases := []struct {
		name, version, branch, commit string
		want                          bool
	}{
		{"release", "2.5.0", "", "", false},
		{"plain go build", "dev", "", "", true},
		{"derived dev version", "dev-abc123", "", "", true},
		{"makefile build", "2.5.0", "main", "abc1234", true},
	}
	for _, c := range cases {
		buildinfo.Version, buildinfo.Branch, buildinfo.Commit = c.version, c.branch, c.commit
		if got := isSourceBuild(); got != c.want {
			t.Errorf("%s: isSourceBuild() = %v, want %v", c.name, got, c.want)
		}
	}
}
