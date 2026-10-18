package commands

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
)

const waitPollInterval = 2 * time.Second

// pollInterval is a variable so tests can shorten it.
var pollInterval = waitPollInterval

type createSessionOptions struct {
	commandFlags
	title, path, branch, program, category, prompt string
	autoYes, oneShot, wait                         bool
	waitTimeout                                    time.Duration
}

// NewCreateSessionCmd creates a session and optionally waits for it to start.
func NewCreateSessionCmd() *cobra.Command {
	var o createSessionOptions
	cmd := &cobra.Command{
		Use:   "create-session --title <title> --path <path>",
		Short: "Create a session on a running server",
		Long: `Create a session. Creation is asynchronous: without --wait the command
prints the new session ID as soon as the server accepts the request, while the
session is still SESSION_STATUS_CREATING.

CI usage note: always pass --wait in automation. Without it, later steer or
get-diff calls can run against a session that is still initializing.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runCreateSession(cmd, &o) },
	}
	o.register(cmd)
	f := cmd.Flags()
	f.StringVar(&o.title, "title", "", "session title (required)")
	f.StringVar(&o.path, "path", "", "repository path (required)")
	f.StringVar(&o.branch, "branch", "", "branch name")
	f.StringVar(&o.program, "program", "", "program to run (e.g. claude, aider)")
	f.StringVar(&o.category, "category", "", "session category")
	f.StringVar(&o.prompt, "prompt", "", "initial prompt")
	f.BoolVar(&o.autoYes, "auto-yes", false, "auto-approve prompts")
	f.BoolVar(&o.oneShot, "one-shot", false, "run a single task then stop")
	f.BoolVar(&o.wait, "wait", false, "block until the session is active")
	f.DurationVar(&o.waitTimeout, "wait-timeout", 60*time.Second, "maximum time --wait blocks while the session is creating")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

func runCreateSession(cmd *cobra.Command, o *createSessionOptions) error {
	ctx := context.Background()
	serverURL := o.serverURL()
	client := NewSessionClient(serverURL)
	resp, err := client.CreateSession(ctx, connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title: o.title, Path: o.path, Branch: o.branch, Program: o.program,
		Category: o.category, Prompt: o.prompt, AutoYes: o.autoYes, OneShot: o.oneShot,
	}))
	if err != nil {
		return rpcError(cmd, err, serverURL)
	}
	sess := resp.Msg.GetSession()
	if o.wait {
		sess, err = waitForActive(ctx, client, sess, o.waitTimeout)
		if err != nil {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			var cerr *connect.Error
			if errors.As(err, &cerr) {
				fmt.Fprintln(cmd.ErrOrStderr(), FormatConnectError(err, serverURL))
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "error: %v\n", err)
			}
			return err
		}
	}
	return printSession(cmd, sess, o.json)
}

func printSession(cmd *cobra.Command, sess *sessionv1.Session, asJSON bool) error {
	if asJSON {
		return printProtoJSON(cmd, sess)
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "session-id: %s\ntitle: %s\nstatus: %s\npath: %s\n",
		sess.GetId(), sess.GetTitle(), sess.GetStatus(), sess.GetActiveDir())
	return nil
}

// waitForActive polls GetSession until the session leaves CREATING. Only
// ACTIVE is success: FAILED/CRASHED/PERMANENTLY_FAILED are terminal errors.
func waitForActive(ctx context.Context, client sessionv1connect.SessionServiceClient, sess *sessionv1.Session, timeout time.Duration) (*sessionv1.Session, error) {
	deadline := time.Now().Add(timeout)
	for {
		if sess.GetStatus() != sessionv1.SessionStatus_SESSION_STATUS_CREATING {
			switch sess.GetStatus() {
			case sessionv1.SessionStatus_SESSION_STATUS_FAILED,
				sessionv1.SessionStatus_SESSION_STATUS_PERMANENTLY_FAILED,
				sessionv1.SessionStatus_SESSION_STATUS_CRASHED:
				return sess, fmt.Errorf("session creation failed: %s reached terminal state %s", sess.GetTitle(), sess.GetStatus())
			}
			return sess, nil
		}
		if !time.Now().Before(deadline) {
			return sess, errors.New("timed out waiting for session to become active")
		}
		time.Sleep(min(pollInterval, time.Until(deadline)))
		resp, err := client.GetSession(ctx, connect.NewRequest(&sessionv1.GetSessionRequest{Id: sess.GetId()}))
		if err != nil {
			return sess, err
		}
		sess = resp.Msg.GetSession()
	}
}
