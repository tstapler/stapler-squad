package backlogstore

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
)

// TestMapAppendActivityNoteCreateError_should_ReturnErrNotFound_When_ErrIsConstraintViolation
// and its sibling below unit-test mapAppendActivityNoteCreateError directly
// (Epic 8.2, Task 8.2.1e / validation.md Gap 1) instead of trying to
// reproduce the delete-between-steps race the FK-violation branch actually
// guards against, which is real but impractical to trigger deterministically.
func TestMapAppendActivityNoteCreateError_should_ReturnErrNotFound_When_ErrIsConstraintViolation(t *testing.T) {
	t.Parallel()
	// *ent.ConstraintError's fields are all unexported, but a zero-value
	// composite literal is still constructible from another package — its
	// Error() string ("ent: constraint failed: ") is irrelevant here, only
	// its type matters to ent.IsConstraintError's errors.As check.
	constraintErr := fmt.Errorf("create failed: %w", &ent.ConstraintError{})

	got := mapAppendActivityNoteCreateError(constraintErr, "item-1")

	require.Error(t, got)
	assert.True(t, errors.Is(got, domain.ErrNotFound), "a constraint-violation error must map to ErrNotFound")
}

func TestMapAppendActivityNoteCreateError_should_WrapPlainError_When_ErrIsNotConstraintViolation(t *testing.T) {
	t.Parallel()
	plain := errors.New("boom")

	got := mapAppendActivityNoteCreateError(plain, "item-1")

	require.Error(t, got)
	assert.False(t, errors.Is(got, domain.ErrNotFound), "a non-constraint error must not be misclassified as ErrNotFound")
	assert.True(t, errors.Is(got, plain), "the original error must remain unwrappable")
}
