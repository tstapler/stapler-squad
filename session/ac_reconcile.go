package session

import (
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// CheckNoNewAcWaivers is the early, user-facing form of the repository's waiver invariant: it
// reports ErrAcWaiverViaUpdate when incoming introduces a waived criterion existing does not
// already hold. The repository re-checks authoritatively inside its transaction.
func CheckNoNewAcWaivers(existing, incoming domain.AcCriteriaJSON) error {
	in, err := domain.ParseAcCriteria(incoming)
	if err != nil {
		return err
	}
	prior, _ := domain.ParseAcCriteria(existing)
	_, _, err = backlogdto.ReconcileAcWaivers(prior, in)
	return err
}
