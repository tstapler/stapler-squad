package commands

import (
	"context"
	"encoding/json"
	"fmt"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// NewGetSessionCmd prints one session as JSON. It keeps encoding/json (enums
// as integers) so existing scripts parsing its output don't break.
func NewGetSessionCmd() *cobra.Command {
	var server string
	cmd := &cobra.Command{
		Use:   "get-session [name]",
		Short: "Get information about a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			serverURL := ResolveServerURL(server)
			resp, err := NewSessionClient(serverURL).GetSession(context.Background(),
				connect.NewRequest(&sessionv1.GetSessionRequest{Id: args[0]}))
			if err != nil {
				return rpcError(cmd, err, serverURL)
			}
			jsonData, err := json.MarshalIndent(resp.Msg.Session, "", "  ")
			if err != nil {
				return fmt.Errorf("could not marshal session data to json: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(jsonData))
			return nil
		},
	}
	cmd.Flags().StringVarP(&server, "server", "s", "", "server URL (default $SSQ_SERVER, then "+defaultServerURL+")")
	return cmd
}
