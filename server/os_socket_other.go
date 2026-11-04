//go:build !linux && !darwin

package server

func somaxconn() (int64, bool)                  { return 0, false }
func listenQueueCounters() (int64, int64, bool) { return 0, 0, false }
func openFDCount() (int64, bool)                { return 0, false }
