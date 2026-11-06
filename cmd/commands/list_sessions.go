package commands

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// NewListSessionsCmd lists sessions via the server API. Unlike `list`, which
// reads local state directly, it needs a running server.
func NewListSessionsCmd() *cobra.Command {
	var f commandFlags
	var status, category, search string
	cmd := &cobra.Command{
		Use:   "list-sessions",
		Short: "List sessions from a running server",
		Long: `List sessions from a running server as a tab-separated table.
Requires a running server; use "list" to read local state when the server is down.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			req := &sessionv1.ListSessionsRequest{}
			switch strings.ToLower(status) {
			case "all", "":
				// Leave Status nil: UNSPECIFIED would not mean "no filter".
			case "active":
				req.Status = sessionv1.SessionStatus_SESSION_STATUS_ACTIVE.Enum()
			case "paused":
				req.Status = sessionv1.SessionStatus_SESSION_STATUS_PAUSED.Enum()
			default:
				return fmt.Errorf("invalid --status %q: want active, paused or all", status)
			}
			if category != "" {
				req.Category = &category
			}
			if search != "" {
				req.SearchQuery = &search
			}
			serverURL := f.serverURL()
			resp, err := NewSessionClient(serverURL).ListSessions(context.Background(), connect.NewRequest(req))
			if err != nil {
				return rpcError(cmd, err, serverURL)
			}
			sessions := resp.Msg.GetSessions()
			if f.json {
				return printSessionsJSON(cmd, sessions)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "ID\tTITLE\tSTATUS\tPATH")
			for _, s := range sessions {
				fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", s.GetId(), s.GetTitle(), s.GetStatus(), s.GetActiveDir())
			}
			return nil
		},
	}
	f.register(cmd)
	cmd.Flags().StringVar(&status, "status", "all", "filter by status: active, paused or all")
	cmd.Flags().StringVar(&category, "category", "", "filter by category")
	cmd.Flags().StringVar(&search, "search", "", "fuzzy search across title, path, branch")
	return cmd
}

func printSessionsJSON(cmd *cobra.Command, sessions []*sessionv1.Session) error {
	items := make([]string, 0, len(sessions))
	for _, s := range sessions {
		b, err := protojson.Marshal(s)
		if err != nil {
			return fmt.Errorf("could not marshal json: %w", err)
		}
		items = append(items, string(b))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "[%s]\n", strings.Join(items, ","))
	return nil
}
