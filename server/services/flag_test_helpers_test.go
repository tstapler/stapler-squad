package services

import "github.com/tstapler/stapler-squad/server/services/featureflags"

// flippedDefaults is testFlagRegistry with the named flags' defaults inverted.
func flippedDefaults(names ...string) *featureflags.Registry {
	flip := map[string]bool{}
	for _, n := range names {
		flip[n] = true
	}
	specs := testFlagRegistry().Specs()
	for i := range specs {
		if flip[specs[i].Name] {
			specs[i].Default = !specs[i].Default
		}
	}
	return featureflags.NewRegistry(specs)
}
