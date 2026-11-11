package main

// growthOfGaps returns the unresolved feature ids that are not already recorded in the committed gap list:
// the list may only shrink (Task 12.5a), so any id returned here is a new, unrecorded gap.
func growthOfGaps(current, recorded []string) []string {
	known := map[string]bool{}
	for _, id := range recorded {
		known[id] = true
	}
	var grown []string
	for _, id := range current {
		if !known[id] {
			grown = append(grown, id)
		}
	}
	return grown
}
