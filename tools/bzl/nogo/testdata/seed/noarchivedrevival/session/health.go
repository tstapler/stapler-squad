// Package session mimics session/health.go, a file in noarchivedrevival's lifecycle set; never import it.
package session

type Instance struct{}

func (i *Instance) Start(first bool) error { return nil }

// Violation revives a session with no IsArchived() check.
func Violation(i *Instance) { _ = i.Start(false) }
