//go:build race

package unfinished

// raceEnabled lets testing.AllocsPerRun assertions skip, not fail, under -race:
// the race detector adds allocations and sync.Pool randomly drops Puts.
const raceEnabled = true
