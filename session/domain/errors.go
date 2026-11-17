package domain

import "errors"

// ErrPreconditionFailed is returned when an optimistic-locking precondition check fails.
var ErrPreconditionFailed = errors.New("precondition failed: concurrent modification detected")

// ErrNotFound is returned when a requested entity does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when an operation would violate a uniqueness constraint.
var ErrConflict = errors.New("conflict")
