package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// quietLoad1 mirrors BZL_MAX_LOAD1 in tools/bzl/baseline/lib.sh.
const quietLoad1 = 2.0

// noisyEnv is the owner-authorised override; the default is to wait for a quiet host.
const noisyEnv = "BZL_ALLOW_NOISY_HOST"

// AllowNoisy reports whether the loud override is on (flag or BZL_ALLOW_NOISY_HOST=1).
func AllowNoisy(flag bool, env string) bool { return flag || env == "1" }

// IsNoisy is true when either end of a build saw a 1-minute load above the quiet limit (unknown load counts as noisy).
func IsNoisy(pre, post float64) bool {
	return pre < 0 || post < 0 || pre > quietLoad1 || post > quietLoad1
}

// NoisyEntry summarises the load and free memory seen by a set of builds, for the inputs file's noisy_inputs map.
func NoisyEntry(bs []Build, source string) map[string]any {
	e := map[string]any{"not_quiet": false, "source": source, "authorisation": "owner, measured under load; never a quiet-host number"}
	var preMax, postMax, memMin float64 = 0, 0, -1
	for _, b := range bs {
		if b.NotQuiet {
			e["not_quiet"] = true
		}
		preMax, postMax = max(preMax, b.LoadPre), max(postMax, b.LoadPost)
		for _, m := range []float64{b.MemAvailPre, b.MemAvailPost} {
			if m >= 0 && (memMin < 0 || m < memMin) {
				memMin = m
			}
		}
	}
	e["load1_pre_max"], e["load1_post_max"], e["mem_avail_gb_min"] = preMax, postMax, memMin
	return e
}

// AnyNoisy is true when at least one build was not quiet.
func AnyNoisy(bs []Build) bool {
	for _, b := range bs {
		if b.NotQuiet {
			return true
		}
	}
	return false
}

// UpdateInputs merges kv into the inputs JSON at path and, when noisy, records entry under noisy_inputs[key] for every key in kv.
func UpdateInputs(path string, kv map[string]any, noisy bool, entry map[string]any) error {
	cur := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &cur); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	side, _ := cur["noisy_inputs"].(map[string]any)
	if side == nil {
		side = map[string]any{}
	}
	for k, v := range kv {
		cur[k] = v
		if noisy {
			side[k] = entry
		} else {
			delete(side, k)
		}
	}
	if len(side) > 0 {
		cur["noisy_inputs"] = side
	} else {
		delete(cur, "noisy_inputs")
	}
	out, err := json.MarshalIndent(cur, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}
