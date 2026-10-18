package commands

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// NewGetDiffCmd prints a session's unified diff.
func NewGetDiffCmd() *cobra.Command {
	var f commandFlags
	cmd := &cobra.Command{
		Use:   "get-diff <session-id>",
		Short: "Print a session's unified diff",
		Long:  "Print the raw unified diff to stdout (empty when there are no changes), suitable for piping to patch or delta.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			serverURL := f.serverURL()
			resp, err := NewSessionClient(serverURL).GetSessionDiff(context.Background(),
				connect.NewRequest(&sessionv1.GetSessionDiffRequest{Id: args[0]}))
			if err != nil {
				return rpcError(cmd, err, serverURL)
			}
			if f.json {
				return printProtoJSON(cmd, resp.Msg)
			}
			fmt.Fprint(cmd.OutOrStdout(), resp.Msg.GetDiffStats().GetContent())
			return nil
		},
	}
	f.register(cmd)
	return cmd
}
