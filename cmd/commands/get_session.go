package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
)

var GetSessionCmd = &cobra.Command{
	Use:   "get-session [name]",
	Short: "Get information about a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := sessionv1connect.NewSessionServiceClient(
			http.DefaultClient,
			"http://localhost:8080", // Assuming the server runs on localhost:8080
		)
		req := &connect.Request[sessionv1.GetSessionRequest]{
			Msg: &sessionv1.GetSessionRequest{
				Id: args[0],
			},
		}
		resp, err := client.GetSession(context.Background(), req)
		if err != nil {
			return fmt.Errorf("could not get session: %w", err)
		}

		jsonData, err := json.MarshalIndent(resp.Msg.Session, "", "  ")
		if err != nil {
			return fmt.Errorf("could not marshal session data to json: %w", err)
		}
		fmt.Println(string(jsonData))

		return nil
	},
}
