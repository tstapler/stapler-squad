// Package services mimics server/services/question_answer_router.go. The
// AnswerRouter resume route never revives by decision (Task 3.4d); the file is
// in lifecycleFiles as a ratchet, so any revival added there needs a guard.
package services

type inst struct{}

func (i *inst) Start(first bool) error { return nil }
func (i *inst) IsArchived() bool       { return false }

func resumeRoute(i *inst) error {
	return nil
}

func sneakyRevive(i *inst) error {
	return i.Start(false) // want `Start\(false\) revives`
}
