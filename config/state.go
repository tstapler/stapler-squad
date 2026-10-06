package config

// StateFileName and InstancesFileName are watched by daemon.RunDaemon (see
// daemon/daemon.go) to detect changes to legacy pre-web-UI state files.
const (
	StateFileName     = "state.json"
	InstancesFileName = "instances.json"
)
