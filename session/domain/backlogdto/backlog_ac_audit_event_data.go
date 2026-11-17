package backlogdto

import (
	"time"
)

// AcAuditEventData is the domain DTO of one AcAuditEvent row.
type AcAuditEventData struct {
	ID               string
	ItemID           string
	ItemPublicID     string
	CriterionIndex   int
	PreviousStatus   string
	NewStatus        string
	Reason           string
	Outcome          string
	ActorClass       string
	ActorSessionUUID string
	SessionVerified  bool
	Source           string
	RemoteAddr       string
	UserAgent        string
	CreatedAt        time.Time
}
