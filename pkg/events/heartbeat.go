package events

import "time"

// HeartbeatInterval is how often every Watch* streaming RPC (WatchSessions,
// WatchBacklogItems, WatchWorkflows, WatchReviewQueue) sends a synthetic
// heartbeat message on an otherwise-idle connection, so a client can tell
// "connected and quiet" apart from a silent stall -- e.g. a proxy
// idle-timeout that stops relaying bytes without ever surfacing an error or
// a close to the client's stream reader. Client-side staleness thresholds
// (web-app/src/lib/hooks/useWatchStream.ts) stay well above this so a couple
// of missed heartbeats, not one, are required before it forces a reconnect.
const HeartbeatInterval = 15 * time.Second
