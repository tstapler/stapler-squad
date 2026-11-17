package workflowengine

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSeedFingerprint_Stable_AndChangesOnEdgeStageLivenessOrGateChange(t *testing.T) {
	t.Parallel()
	require.Equal(t, SeedFingerprint(), SeedFingerprint())
	base := builtInSeedFingerprintInput().hash()
	require.Equal(t, SeedFingerprint(), base)

	mutations := map[string]func(c *stageConfigFingerprint){
		"remove one edge": func(c *stageConfigFingerprint) { c.Edges = c.Edges[1:] },
		"disable one stage": func(c *stageConfigFingerprint) {
			c.Stages[0] = stageRow{Slug: "idea", Name: "Idea", Entry: true}.line()
		},
		"add a liveness row": func(c *stageConfigFingerprint) {
			c.Liveness = append(c.Liveness, "idea|mode=nil|kind=duration_budget|expected=1000|margin=nil|noprogress=nil|threshold=nil|lookback=nil|enabled=true")
		},
		"add one gate": func(c *stageConfigFingerprint) {
			c.Gates = append(c.Gates, "idea->ready|kind=human_approval|stateful=true|order=0|enabled=true|config=null")
		},
	}
	for name, mutate := range mutations {
		in := builtInSeedFingerprintInput()
		mutate(&in)
		require.NotEqual(t, base, in.hash(), name)
	}

	// Changing one liveness value changes the hash relative to the row it edits.
	a, b := builtInSeedFingerprintInput(), builtInSeedFingerprintInput()
	a.Liveness = []string{"idea|mode=nil|kind=duration_budget|expected=1000|margin=nil|noprogress=nil|threshold=nil|lookback=nil|enabled=true"}
	b.Liveness = []string{strings.Replace(a.Liveness[0], "expected=1000", "expected=2000", 1)}
	require.NotEqual(t, a.hash(), b.hash())
}
