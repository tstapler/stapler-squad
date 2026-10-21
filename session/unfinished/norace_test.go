//go:build !race

package unfinished

// raceEnabled is false in ordinary builds, where AllocsPerRun gates run.
const raceEnabled = false
