package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// Budgets is tools/promo/budgets.json, the single home of every size, time and
// path-set limit.
type Budgets struct {
	ClipMaxBytes              int64    `json:"clipMaxBytes"`
	ImageMaxBytes             int64    `json:"imageMaxBytes"`
	DistMaxBytes              int64    `json:"distMaxBytes"`
	RecordWallClockMaxMinutes int      `json:"recordWallClockMaxMinutes"`
	ClipMaxDurationMs         int64    `json:"clipMaxDurationMs"`
	RecordInputPaths          []string `json:"recordInputPaths"`
}

// ReadBudgets loads and validates a budgets file; unknown keys and non-positive
// limits are errors.
func ReadBudgets(path string) (Budgets, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Budgets{}, err
	}
	var b Budgets
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		return Budgets{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if b.ClipMaxBytes <= 0 || b.ImageMaxBytes <= 0 || b.DistMaxBytes <= 0 ||
		b.RecordWallClockMaxMinutes <= 0 || b.ClipMaxDurationMs <= 0 || len(b.RecordInputPaths) == 0 {
		return Budgets{}, fmt.Errorf("%s: every budget must be positive and recordInputPaths non-empty", path)
	}
	return b, nil
}
