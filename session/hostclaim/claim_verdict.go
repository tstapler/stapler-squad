package hostclaim

// ClaimVerdictKind is the three-way outcome of a cross-host claim check.
type ClaimVerdictKind int

const (
	// ClaimUnclaimed: no other host is known to hold the URL.
	ClaimUnclaimed ClaimVerdictKind = iota
	// ClaimHeldByOther: a different host holds the URL; ClaimVerdict.Record says which.
	ClaimHeldByOther
	// ClaimCheckIndeterminate: at least one peer was unreachable and none reported
	// a claim, so "unclaimed" cannot be confirmed. Callers proceed optimistically.
	ClaimCheckIndeterminate
)

// ClaimVerdict is a claim check result. Build it with the New*Verdict
// constructors so Kind and Record cannot be mismatched.
type ClaimVerdict struct {
	Kind   ClaimVerdictKind
	Record ClaimRecord // meaningful only for ClaimHeldByOther
}

func NewUnclaimedVerdict() ClaimVerdict { return ClaimVerdict{Kind: ClaimUnclaimed} }

func NewIndeterminateVerdict() ClaimVerdict { return ClaimVerdict{Kind: ClaimCheckIndeterminate} }

func NewHeldByOtherVerdict(record ClaimRecord) ClaimVerdict {
	return ClaimVerdict{Kind: ClaimHeldByOther, Record: record}
}
