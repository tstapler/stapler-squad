package commands

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// NewSteerCmd injects a message into a running session.
func NewSteerCmd() *cobra.Command {
	var f commandFlags
	var message string
	cmd := &cobra.Command{
		Use:   "steer <session-id> --message <text>",
		Short: "Send a steering message to a running session",
		Long: `Send a steering message to a session. On an autonomous session the message
enters the driver's command queue; on any other session it is typed directly
into the terminal pane as if the user pressed Enter. Sessions the server
refuses to steer (e.g. hidden sessions) fail with the server's message.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			serverURL := f.serverURL()
			resp, err := NewSessionClient(serverURL).UpdateSession(context.Background(),
				connect.NewRequest(&sessionv1.UpdateSessionRequest{Id: args[0], SteerMessage: &message}))
			if err != nil {
				return rpcError(cmd, err, serverURL)
			}
			if f.json {
				return printProtoJSON(cmd, resp.Msg)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "ok")
			return nil
		},
	}
	f.register(cmd)
	cmd.Flags().StringVarP(&message, "message", "m", "", "message to send (required)")
	_ = cmd.MarkFlagRequired("message")
	return cmd
}
