package inbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadFixture(t *testing.T, name string) []Item {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	require.NoError(t, err)
	var wrapper struct {
		Items []Item `json:"items"`
	}
	require.NoError(t, json.Unmarshal(raw, &wrapper))
	return wrapper.Items
}

// Fixtures mirror gh-signal's normalize.rs output (crates/gh-signal-core/src/inbox/normalize.rs);
// review state is details.state, lowercased.
func TestDecode_should_ParseGoldenEnvelopeFromGhSignal_When_EachKindFixtureLoaded(t *testing.T) {
	cases := []struct {
		file, kind, sourceType string
		nilActor, nilHead      bool
		detailKey, detailValue string
	}{
		{"comment", "comment", "issue_comment", false, true, "comment_id", "456"},
		{"review", "review", "pull_request_review", false, false, "state", `"changes_requested"`},
		{"review_comment", "review_comment", "pull_request_review_comment", false, false, "state", "null"},
		{"pull_request", "pull_request", "pull_request", false, false, "state", `"open"`},
		{"ci", "ci", "check_run", true, false, "conclusion", `"failure"`},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			items := loadFixture(t, tc.file)
			require.Len(t, items, 1)
			it := items[0]

			// Three distinct IDs: ack uses the item, dedup the event, unsubscribe the subscription.
			assert.Equal(t, "inb_"+tc.file, it.ID)
			assert.Equal(t, "sub_1", it.SubscriptionID)
			assert.Contains(t, it.Event.ID, "evt_")
			assert.NotEqual(t, it.ID, it.Event.ID)
			assert.False(t, it.CreatedAt.IsZero())

			e := it.Event
			assert.Equal(t, 1, e.Version)
			assert.Equal(t, tc.kind, e.Kind)
			assert.Equal(t, tc.sourceType, e.SourceEventType)
			assert.Equal(t, "acme-corp/widget-api", e.Repository)
			assert.Equal(t, 123, e.PullRequest)
			assert.Equal(t, "untrusted", e.Trust)
			assert.Len(t, e.Fingerprint, 64)
			assert.False(t, e.OccurredAt.IsZero())
			assert.Equal(t, tc.nilActor, e.Actor == "", "null actor decodes to empty string")
			assert.Equal(t, tc.nilHead, e.HeadSHA == nil)

			var details map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(e.Details, &details))
			assert.Equal(t, tc.detailValue, string(details[tc.detailKey]))
		})
	}
}

func TestDecode_should_IgnoreUnknownFields_When_FixtureCarriesFutureFields(t *testing.T) {
	// comment.json carries an extra top-level "future_field" on the event.
	items := loadFixture(t, "comment")
	require.Len(t, items, 1)
	assert.Equal(t, "comment", items[0].Event.Kind)
}
