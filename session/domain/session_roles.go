package domain

// ItemSession.session_role values the persistence layer filters on. The
// session/instance constants of the same names re-export these so there is one
// definition.
const (
	SessionRoleWork      = "work"
	SessionRoleReview    = "review"
	SessionRoleJulesWork = "jules_work"
	SessionRoleDiagnose  = "diagnose"
)
