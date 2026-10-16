package services

import "github.com/google/uuid"

// deployID is fixed for the process lifetime and changes on every restart or redeploy, so a client
// can tell "server restarted, seq reset" apart from a genuine seq anomaly (see SessionEvent.deploy_id).
var deployID = uuid.NewString() //nolint:gochecknoglobals

// DeployID returns this server process's deploy ID.
func DeployID() string { return deployID }
