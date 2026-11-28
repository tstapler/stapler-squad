package services

import "time"

// ServerStartTime records when this process started. Debug snapshots report uptime
// from it and the backlog tombstone checks grant restart grace from it.
var ServerStartTime = time.Now()
