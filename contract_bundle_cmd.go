package main

import (
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tstapler/stapler-squad/server/integrations/contractarchive"
)

const defaultContractDir = "contracts/webhook-management/v1"

// newContractBundleCmd builds the deterministic release archive of the webhook-management
// contract. Offline and read-only apart from the output file; the archive is attested separately
// by the release pipeline, never signed here.
func newContractBundleCmd() *cobra.Command {
	var dir, out string
	cmd := &cobra.Command{
		Use:   "contract-bundle",
		Short: "Build the webhook-management contract release archive",
		Long: "Packs the pinned files of the webhook-management contract into a deterministic tar.gz " +
			"with a root MANIFEST.json, and prints its SHA-256 and size.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			archive, err := contractarchive.Build(dir)
			if err != nil {
				return err
			}
			if _, err := contractarchive.Verify(archive); err != nil {
				return fmt.Errorf("built archive failed its own verification: %w", err)
			}
			if err := os.WriteFile(out, archive, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", out, err)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%x  %s (%d bytes)\n", sha256.Sum256(archive), out, len(archive))
			return err
		},
	}
	cmd.Flags().StringVar(&dir, "dir", defaultContractDir, "directory holding the contract files")
	cmd.Flags().StringVar(&out, "out", "", "path of the tar.gz to write (required)")
	_ = cmd.MarkFlagRequired("out") //nolint:errcheck // only errors for an unknown flag name
	return cmd
}
