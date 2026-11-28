//go:build !race

package linediff

// raceEnabled is false in ordinary builds, where AllocsPerRun gates run.
const raceEnabled = false
