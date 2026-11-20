package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// FeatureFlagDefault resolved against a process-wide registry; readers now take their registry explicitly.
func TestServices_ShouldNotReferenceRemovedFlagGlobals(t *testing.T) {
	t.Parallel()
	needles := []string{"FeatureFlagDefault" + "(", "featureflags.Current" + "(", "featureflags.SetCurrent" + "("}
	var hits []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for _, n := range needles {
			if strings.Contains(string(b), n) {
				hits = append(hits, path+": "+n)
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.Empty(t, hits)
}
