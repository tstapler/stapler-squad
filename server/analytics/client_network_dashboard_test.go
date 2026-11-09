package analytics

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The dashboard and docs name series by their VictoriaMetrics spelling; this pins them to the
// documented diagnostic query so a rename in either place fails here instead of going blank.
func TestDashboard_ShouldParseAndHaveTerminalInputPanel_When_JsonLoaded(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../docs/observability/grafana/dashboards/client-network.json")
	require.NoError(t, err)
	var d struct {
		Panels []struct {
			Title   string `json:"title"`
			Targets []struct {
				Expr string `json:"expr"`
			} `json:"targets"`
		} `json:"panels"`
	}
	require.NoError(t, json.Unmarshal(raw, &d))

	var exprs []string
	for _, p := range d.Panels {
		if p.Title == "Terminal input" {
			for _, tg := range p.Targets {
				exprs = append(exprs, tg.Expr)
			}
		}
	}
	require.NotEmpty(t, exprs, `panel titled "Terminal input" with queries`)
	joined := strings.Join(exprs, "\n")
	for _, series := range []string{
		`client_input_total{event="emitted"}`,
		`terminal_input_frames_total{kind="text",stage="received"`,
		`terminal_input_frames_total{kind="text",stage="forwarded"`,
	} {
		assert.Contains(t, joined, series)
	}

	titles := make([]string, 0, len(d.Panels))
	var anomalyExprs string
	for _, p := range d.Panels {
		titles = append(titles, p.Title)
		if p.Title == "Phone input anomalies and drops" {
			for _, tg := range p.Targets {
				anomalyExprs += tg.Expr
			}
		}
	}
	assert.Contains(t, titles, "Phone input anomalies and drops")
	assert.Contains(t, anomalyExprs, "client_ime_anomaly_total")
	assert.Contains(t, anomalyExprs, "dropped_disconnected")
}

func TestDoc_ShouldListCmInputAndImeAnomalyRowsWithNameMapping_When_Grepped(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../docs/reference/client-network-telemetry.md")
	require.NoError(t, err)
	doc := string(raw)
	for _, want := range []string{
		"`client_input_total`", "`cm.input`", "`client_ime_anomaly_total`", "`cm.ime_anomaly`",
		"input_dropped_disconnected_total", "ime_event_sequence_unrecognized_total",
	} {
		assert.Contains(t, doc, want)
	}
}
