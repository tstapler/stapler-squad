package session

import (
	"context"

	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/migrations"
)

// The per-migration tests drive the leaf package's exported run functions with
// the same Env NewEntRepository builds.
func migrationEnv(er *EntRepository) migrations.Env {
	return migrations.Env{Client: er.Client, Driver: er.driver, BackfillPublicIDs: er.BackfillBacklogItemPublicIDs}
}

func runBacklogItemUpdatedAtUTCBackfill(ctx context.Context, er *EntRepository) error {
	return migrations.BacklogItemUpdatedAtUTC(ctx, migrationEnv(er))
}

func runWorkflowUpdatedAtUTCBackfill(ctx context.Context, er *EntRepository) error {
	return migrations.WorkflowUpdatedAtUTC(ctx, migrationEnv(er))
}

func runGitHubPRURLBackfill(ctx context.Context, er *EntRepository) error {
	return migrations.PRURLBackfill(ctx, migrationEnv(er), sessionPRURL)
}

func runBacklogItemRepoPathCanonicalizationBackfill(ctx context.Context, er *EntRepository) error {
	return migrations.BacklogItemRepoPathCanonicalization(ctx, migrationEnv(er), instance.ResolveMainRepoRoot)
}

func runBacklogItemLegacyImportExternalBackfill(ctx context.Context, er *EntRepository) error {
	return migrations.BacklogItemLegacyImportExternal(ctx, migrationEnv(er))
}
