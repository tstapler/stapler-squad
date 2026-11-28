package actuator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tstapler/stapler-squad/session/nudgeoutcomes"
)

func TestHandleActuatorNudgeOutcomes_ServesTotalsAndSince(t *testing.T) {
	rec := nudgeoutcomes.NewNudgeOutcomeRecorder("", nil)
	defer nudgeoutcomes.SwapNudgeOutcomeRecorder(rec)()
	rec.Record(nudgeoutcomes.NudgeKindStall, nudgeoutcomes.NudgeOutcomeDelivered, "S1")

	w := httptest.NewRecorder()
	NudgeOutcomes(w, httptest.NewRequest(http.MethodGet, "/actuator/nudge-outcomes", nil))

	var body nudgeoutcomes.NudgeOutcomeReport
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v: %s", err, w.Body.String())
	}
	if len(body.Totals) != 1 || body.Totals[0].Kind != "stall" || body.Totals[0].Outcome != "delivered" || body.Totals[0].Value != 1 {
		t.Fatalf("totals = %+v", body.Totals)
	}
	if body.Since == nil {
		t.Fatal("since must be set after the first record")
	}
}
