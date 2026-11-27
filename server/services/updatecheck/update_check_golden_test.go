package updatecheck

// Differential golden for the update-check JSON over a flag, build and fetch
// matrix, recorded from origin/main before the move.

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var updateUpdateCheckGolden = flag.Bool("update-update-check-golden", false, "rewrite testdata/update_check.golden")

func TestUpdateCheckGolden(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	for _, current := range []string{"1.2.0", "2.0.0", "dev"} {
		for _, source := range []bool{false, true} {
			enabled, fail, calls := false, false, 0
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			svc := NewUpdateCheckService(UpdateCheckDeps{
				CurrentVersion: current,
				IsSourceBuild:  source,
				Enabled:        func() bool { return enabled },
				Now:            func() time.Time { return now },
				Fetch: func(context.Context) (*ReleaseInfo, error) {
					calls++
					if fail {
						return nil, errors.New("offline")
					}
					return &ReleaseInfo{Version: "1.9.0", URL: "https://example.invalid/r"}, nil
				},
			})
			step := func(name string, advance time.Duration) {
				now = now.Add(advance)
				did := svc.MaybeCheck(context.Background())
				b, err := json.Marshal(svc.Current())
				require.NoError(t, err)
				fmt.Fprintf(&out, "%s source=%t %s did=%t calls=%d %s\n", current, source, name, did, calls, b)
			}
			step("disabled", 0)
			enabled = true
			step("first", time.Hour)
			step("within-ttl", time.Hour)
			fail = true
			step("error", 7*time.Hour)
			enabled = false
			step("off-again", 7*time.Hour)
		}
	}
	nilDeps := NewUpdateCheckService(UpdateCheckDeps{CurrentVersion: "1.0.0"})
	fmt.Fprintf(&out, "nil-deps did=%t %+v\n", nilDeps.MaybeCheck(context.Background()), nilDeps.Current())

	const path = "testdata/update_check.golden"
	if *updateUpdateCheckGolden {
		require.NoError(t, os.WriteFile(path, []byte(out.String()), 0o600))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), out.String())
}
