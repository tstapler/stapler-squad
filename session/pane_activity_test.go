package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPaneActivitySettled(t *testing.T) {
	t.Parallel()
	capture := time.Unix(1700000010, 500_000_000)
	cases := map[string]struct {
		activity time.Time
		want     bool
	}{
		"zero is unknown":               {time.Time{}, false},
		"earlier second is trusted":     {time.Unix(1700000009, 0), true},
		"same second may still change":  {time.Unix(1700000010, 0), false},
		"future (clock skew) untrusted": {time.Unix(1700000011, 0), false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, paneActivitySettled(tc.activity, capture))
		})
	}
}
