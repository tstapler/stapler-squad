package schema_test

// migration_should_be_reversible is the migration test named in
// project_plans/async-session-creation/implementation/validation.md's
// "Migration Test Design (Step 5)" section, for the three async-session-
// creation ent schema fields added to session/ent/schema/session.go:
// creation_epoch, failure_reason, and creation_progress_updated_at.
//
// Per that section, this repo's ent schema is hand-written and its generated
// output (session/ent/*.go) is never committed -- there is no traditional
// SQL up/down migration file to run. The "migration" here is: (a) the new
// schema fields generate correctly and session/ent builds against them
// (items 1-2 of the spec, asserted below at both compile time -- this file
// only compiles if session/ent exposes the expected accessors -- and
// runtime, via a real in-memory-SQLite round trip); (b) an existing
// persisted row from before this deploy loads with these fields at their
// zero values, since the plan's Migration Plan section calls for no
// backfill. Items 3-4 (revert the schema commit, regenerate, confirm
// go build/go test still pass with the fields absent) are process steps for
// a human running `git revert` + `make ent-gen`, not something a Go test
// run against the current tree can assert -- there is nothing to execute
// once the fields genuinely don't exist in the checked-out schema.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

// TestMigration_should_BeReversible_When_CreationFieldsAreAtZeroValues covers
// spec items 1-2: the schema generates correctly (go build already proved
// this by the time this test runs) and a row that never sets
// creation_epoch/failure_reason/creation_progress_updated_at -- simulating a
// row persisted before this deploy -- round-trips their documented zero
// values (0, "", nil) rather than erroring or silently defaulting to
// something else.
func TestMigration_should_BeReversible_When_CreationFieldsAreAtZeroValues(t *testing.T) {
	repo := session.NewTestEntRepository(t)
	client := repo.GetEntClient()
	require.NotNil(t, client, "NewTestEntRepository must expose a live ent client")

	ctx := context.Background()

	// A minimal row, deliberately never touching CreationEpoch/FailureReason/
	// CreationProgressUpdatedAt -- exactly what a pre-deploy row (created
	// before these fields existed) looks like once the schema migration has
	// run against its database.
	created, err := client.Session.Create().
		SetTitle("migration-test-pre-deploy-row").
		SetPath("/tmp/migration-test-pre-deploy-row").
		SetProgram("claude").
		SetStatus(0).
		Save(ctx)
	require.NoError(t, err)

	// Re-fetch by ID to prove the values are actually queryable from the
	// database, not just echoed back from the in-memory create result.
	fetched, err := client.Session.Get(ctx, created.ID)
	require.NoError(t, err)

	assert.Equal(t, uint64(0), fetched.CreationEpoch,
		"creation_epoch must round-trip its documented zero-value default of 0")
	assert.Equal(t, "", fetched.FailureReason,
		"failure_reason must round-trip its documented zero-value default of empty string")
	assert.Nil(t, fetched.CreationProgressUpdatedAt,
		"creation_progress_updated_at is Optional().Nillable(); a row that never sets it must read back nil, not a zero time.Time")
}
