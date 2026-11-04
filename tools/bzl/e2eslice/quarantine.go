package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// QuarantineRow is one tests/e2e/quarantine.json entry (ADR-009: a spec failing twice in the last 10 main runs).
type QuarantineRow struct {
	Spec   string `json:"spec"`
	Owner  string `json:"owner"`
	Issue  string `json:"issue"`
	Date   string `json:"date"`
	Expiry string `json:"expiry"`
}

// checkQuarantine enforces Task 12.5d: each row has an owner, an issue and an expiry within 30 days of its date,
// and the generated BUILD file uses neither flaky = True nor --flaky_test_attempts.
func checkQuarantine(rows []QuarantineRow, buildText string) []string {
	var bad []string
	for _, r := range rows {
		if r.Owner == "" || r.Issue == "" {
			bad = append(bad, r.Spec+": owner and issue are required")
		}
		d, e1 := time.Parse("2006-01-02", r.Date)
		x, e2 := time.Parse("2006-01-02", r.Expiry)
		switch {
		case e1 != nil || e2 != nil:
			bad = append(bad, r.Spec+": date and expiry must be YYYY-MM-DD")
		case x.After(d.AddDate(0, 0, 30)):
			bad = append(bad, fmt.Sprintf("%s: expiry %s is more than 30 days after %s", r.Spec, r.Expiry, r.Date))
		case x.Before(time.Now().AddDate(0, 0, 0)) && time.Since(x) > 0:
			bad = append(bad, r.Spec+": quarantine expired on "+r.Expiry)
		}
	}
	if strings.Contains(buildText, "flaky = True") || strings.Contains(buildText, "--flaky_test_attempts") {
		bad = append(bad, "e2e targets use flaky = True or --flaky_test_attempts (banned, ADR-009)")
	}
	return bad
}

func readQuarantine(path string) ([]QuarantineRow, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rows []QuarantineRow
	return rows, json.Unmarshal(b, &rows)
}
