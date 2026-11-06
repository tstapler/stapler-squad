package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

var integrationPrincipalRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

const (
	principalFlag  = "principal"
	allowedDirFlag = "allowed-dir"
)

// openStateRepository opens the database of the state directory this process resolves,
// the same <config dir>/sessions.db the server uses. It must not use session.NewEntRepository's
// bare default (~/.stapler-squad/sessions.db): that ignores STAPLER_SQUAD_INSTANCE and workspace
// modes, so a credential would land in a different instance's database than the one serving it.
func openStateRepository() (*session.EntRepository, string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, "", fmt.Errorf("resolve state directory: %w", err)
	}
	repo, err := session.NewEntRepository(session.WithDatabasePath(filepath.Join(configDir, "sessions.db")))
	if err != nil {
		return nil, "", fmt.Errorf("open repository: %w", err)
	}
	return repo, configDir, nil
}

// newIntegrationCredentialCmd builds the command group managing the machine credentials
// external integrations (e.g. gh-signal) use against the webhook-management API. Offline by
// design: minting a credential is an operator action on the machine that owns the state
// directory, never something reachable over the network.
func newIntegrationCredentialCmd() *cobra.Command {
	group := &cobra.Command{
		Use:   "integration-credential",
		Short: "Manage credentials for the webhook-management API",
	}
	group.AddCommand(newIssueCredentialCmd(), newRevokeCredentialCmd())
	return group
}

func newIssueCredentialCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue --principal <id> --allowed-dir <dir>",
		Short: "Issue a credential bound to this state directory and a directory root",
		Long: "Prints the token to stdout exactly once; it cannot be recovered afterwards.\n" +
			"The credential can only register webhooks whose target directory is <dir> or beneath it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			principal, _ := cmd.Flags().GetString(principalFlag)
			allowedDir, _ := cmd.Flags().GetString(allowedDirFlag)
			if !integrationPrincipalRe.MatchString(principal) {
				return fmt.Errorf("--%s must be 1-64 characters of [A-Za-z0-9._-]", principalFlag)
			}
			if !filepath.IsAbs(allowedDir) || filepath.Clean(allowedDir) != allowedDir {
				return fmt.Errorf("--%s must be an absolute, clean path", allowedDirFlag)
			}
			if info, err := os.Stat(allowedDir); err != nil || !info.IsDir() {
				return fmt.Errorf("--%s %q is not an existing directory", allowedDirFlag, allowedDir)
			}
			repo, configDir, err := openStateRepository()
			if err != nil {
				return err
			}
			defer repo.Close()

			workspaceID := services.WebhookWorkspaceID(configDir)
			token, err := repo.IssueIntegrationCredential(cmd.Context(), session.IntegrationCredentialInfo{
				PrincipalID: principal, WorkspaceID: workspaceID, AllowedDirRoot: allowedDir,
			})
			if err != nil {
				return fmt.Errorf("issue credential: %w", err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Issued credential for principal %q (workspace %s, directory %s).\n"+
				"Store the token now; it is shown only once.\n", principal, workspaceID, allowedDir)
			fmt.Fprintln(cmd.OutOrStdout(), token)
			return nil
		},
	}
	cmd.Flags().String(principalFlag, "", "Stable identifier for the integration client (e.g. gh-signal)")
	cmd.Flags().String(allowedDirFlag, "", "Absolute directory that registrations made with this credential may target")
	return cmd
}

func newRevokeCredentialCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke --principal <id>",
		Short: "Revoke a principal's credential (its registrations are left in place)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			principal, _ := cmd.Flags().GetString(principalFlag)
			if !integrationPrincipalRe.MatchString(principal) {
				return fmt.Errorf("--%s must be 1-64 characters of [A-Za-z0-9._-]", principalFlag)
			}
			repo, _, err := openStateRepository()
			if err != nil {
				return err
			}
			defer repo.Close()
			if err := repo.RevokeIntegrationCredential(cmd.Context(), principal); err != nil {
				return fmt.Errorf("revoke credential: %w", err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Revoked credential for principal %q.\n", principal)
			return nil
		},
	}
	cmd.Flags().String(principalFlag, "", "Principal whose credential to revoke")
	return cmd
}
