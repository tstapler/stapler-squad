package commands

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	defaultServerURL = "http://localhost:8543/api"
	serverEnvVar     = "SSQ_SERVER"
	requestTimeout   = 15 * time.Second
)

// ResolveServerURL returns the server base URL: the --server flag value if
// set, else $SSQ_SERVER, else the default local instance.
func ResolveServerURL(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if env := os.Getenv(serverEnvVar); env != "" {
		return env
	}
	return defaultServerURL
}

// NewSessionClient builds a SessionService client with a bounded per-request
// timeout so a wedged server can't hang a script (http.DefaultClient has none).
func NewSessionClient(serverURL string) sessionv1connect.SessionServiceClient {
	return sessionv1connect.NewSessionServiceClient(&http.Client{Timeout: requestTimeout}, serverURL)
}

// FormatConnectError renders an RPC error for stderr, with a hint when the
// server is unreachable and the server's verbatim message otherwise.
func FormatConnectError(err error, serverURL string) string {
	var cerr *connect.Error
	if !errors.As(err, &cerr) {
		return fmt.Sprintf("error: %v", err)
	}
	if cerr.Code() == connect.CodeUnavailable {
		return fmt.Sprintf("error: server unreachable at %s — is stapler-squad running?\ndetail: %s", serverURL, cerr.Message())
	}
	return fmt.Sprintf("error: %s", cerr.Message())
}

// MarshalProtoJSON marshals with protojson so enums appear as string names.
func MarshalProtoJSON(msg proto.Message) ([]byte, error) {
	return protojson.MarshalOptions{Multiline: true}.Marshal(msg)
}

// commandFlags are the flags shared by every server-facing subcommand.
type commandFlags struct {
	server string
	json   bool
}

func (f *commandFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&f.server, "server", "s", "", "server URL (default $SSQ_SERVER, then "+defaultServerURL+")")
	cmd.Flags().BoolVar(&f.json, "json", false, "emit JSON with enum values as string names")
}

func (f *commandFlags) serverURL() string { return ResolveServerURL(f.server) }

// rpcError converts an RPC failure into a stderr-ready error and silences
// cobra's own usage/error printing so the message appears once.
func rpcError(cmd *cobra.Command, err error, serverURL string) error {
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	fmt.Fprintln(cmd.ErrOrStderr(), FormatConnectError(err, serverURL))
	return err
}

func printProtoJSON(cmd *cobra.Command, msg proto.Message) error {
	b, err := MarshalProtoJSON(msg)
	if err != nil {
		return fmt.Errorf("could not marshal json: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(b))
	return nil
}
