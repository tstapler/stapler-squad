//go:build !linux && !darwin

package telemetry

func readListenCounters() (map[string]int64, bool) { return nil, false }

func readSomaxconn() (int64, bool) { return 0, false }
