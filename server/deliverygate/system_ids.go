package deliverygate

import "strings"

// systemIDs is the closed set of non-session identifiers producers pass in the
// session slot of NewNotificationEvent (Spike 1.3d). Anything else not found in
// the index is Unresolved, never NotASession: a real session title is
// indistinguishable from any other string, so only this set and an item_id
// stamp count as positively "not a session".
var systemIDs = map[string]struct{}{
	"fork-pressure":      {}, // server/server.go fork-pressure notifier
	"tmux-server":        {}, // server/server.go tmux-server notifier
	"system":             {}, // memory_pressure_notifier.go
	"backlog-quota-gate": {}, // quotaGateNotifierKey
}

// systemIDPrefixes are the prefixed forms ("bulk-reset:<scope>").
var systemIDPrefixes = []string{"bulk-reset:"}

// IsSystemID reports whether id is a known non-session producer identifier.
func IsSystemID(id string) bool {
	if _, ok := systemIDs[id]; ok {
		return true
	}
	for _, p := range systemIDPrefixes {
		if strings.HasPrefix(id, p) {
			return true
		}
	}
	return false
}
