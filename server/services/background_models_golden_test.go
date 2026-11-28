package services

// Differential golden for the background-model RPCs, recorded from origin/main
// before they moved into a leaf. It drives SessionService, so it also pins the
// delegators.

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

var updateBackgroundModelsGolden = flag.Bool("update-background-models-golden", false, "rewrite testdata/background_models.golden")

func TestBackgroundModelsGolden(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc := &SessionService{}
	var out strings.Builder
	record := func(step string, msg proto.Message, err error) {
		if err != nil {
			fmt.Fprintf(&out, "%s err code=%v %v\n", step, connect.CodeOf(err), err)
			return
		}
		raw, merr := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(msg)
		require.NoError(t, merr)
		var v any
		require.NoError(t, json.Unmarshal(raw, &v))
		norm, merr := json.Marshal(v)
		require.NoError(t, merr)
		fmt.Fprintf(&out, "%s %s\n", step, norm)
	}
	get := func(step string) {
		resp, err := svc.GetBackgroundModels(context.Background(), connect.NewRequest(&sessionv1.GetBackgroundModelsRequest{}))
		require.NoError(t, err)
		record(step, resp.Msg, nil)
	}
	update := func(step string, settings *sessionv1.BackgroundModelsProto) {
		resp, err := svc.UpdateBackgroundModels(context.Background(),
			connect.NewRequest(&sessionv1.UpdateBackgroundModelsRequest{Settings: settings}))
		if err != nil {
			record(step, nil, err)
			return
		}
		record(step, resp.Msg, nil)
	}

	get("get-default")
	update("update-nil", nil)
	update("update-unknown-feature", &sessionv1.BackgroundModelsProto{Features: map[string]string{"nope": "haiku"}})
	update("update-flag-like-model", &sessionv1.BackgroundModelsProto{Features: map[string]string{"handoff-summary": "--x"}})
	update("update-unknown-stage", &sessionv1.BackgroundModelsProto{Stages: map[string]string{"triage": "haiku"}})
	update("update-bad-effort", &sessionv1.BackgroundModelsProto{Effort: "turbo"})
	update("update-ok", &sessionv1.BackgroundModelsProto{
		Features: map[string]string{"handoff-summary": " sonnet ", "summarize": ""},
		Stages:   map[string]string{"review": "haiku"},
		Effort:   " Medium ",
	})
	get("get-after")

	path := filepath.Join("testdata", "background_models.golden")
	if *updateBackgroundModelsGolden {
		require.NoError(t, os.WriteFile(path, []byte(out.String()), 0o600))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), out.String())
}
