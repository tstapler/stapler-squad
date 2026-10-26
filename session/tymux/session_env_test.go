package tymux

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/session/launchcmd"
)

// Precedence under test (lowest to highest): paneColorEnv, AppendEnv, command env.
func TestTymuxGRPCSession_Start_MergesAppendedEnv(t *testing.T) {
	t.Setenv("TERM", "")
	t.Setenv("COLORTERM", "")
	cmdEnv, err := launchcmd.NewEnvVar("DISPLAY", ":from-command")
	require.NoError(t, err)
	color := map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor"}
	with := func(extra map[string]string) map[string]string {
		out := map[string]string{}
		for k, v := range color {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	tests := []struct {
		name    string
		appends [][]string
		command launchcmd.LaunchCommand
		want    map[string]string
	}{
		{"nothing appended: color env only", nil, nil, color},
		{"appended env reaches the request", [][]string{{"DISPLAY=:99"}}, nil, with(map[string]string{"DISPLAY": ":99"})},
		{"value may contain '='", [][]string{{"CDP_URL=http://h:1/?a=b"}}, nil, with(map[string]string{"CDP_URL": "http://h:1/?a=b"})},
		{"malformed entries are ignored", [][]string{{"NOEQUALS", "=novalue", "OK=1"}}, nil, with(map[string]string{"OK": "1"})},
		{"repeated key: last append wins", [][]string{{"DISPLAY=:1"}, {"DISPLAY=:2"}}, nil, with(map[string]string{"DISPLAY": ":2"})},
		{"appended env overrides pane color env", [][]string{{"TERM=vt100"}}, nil, with(map[string]string{"TERM": "vt100"})},
		{
			"command env overrides appended env", [][]string{{"DISPLAY=:99"}},
			launchcmd.ExecCommand{Env: []launchcmd.EnvVar{cmdEnv}, Argv: []launchcmd.Arg{launchcmd.Word("claude")}},
			with(map[string]string{"DISPLAY": ":from-command"}),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			var gotReq *v1.CreateSessionRequest
			sess := NewTymuxGRPCSession(captureCreateSessionRequest(dir, &gotReq))
			sess.SetCommand(tc.command)
			for _, env := range tc.appends {
				sess.AppendEnv(env...)
			}

			require.NoError(t, sess.Start(dir))

			assert.Equal(t, tc.want, gotReq.GetEnv())
		})
	}
}
