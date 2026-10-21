package server

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/unfinished"
)

// shadowVerifyEveryEnv overrides the shadow-verify sampling rate (1 in N scans;
// 0 turns it off). Env only: a config key would also need a flag of the same
// name (config keys must equal flag names).
const shadowVerifyEveryEnv = "STAPLER_SQUAD_SHADOW_VERIFY_EVERY"

// parseShadowVerifyEvery reads shadowVerifyEveryEnv through getenv. set is false
// for an unset or empty variable; err is non-nil for a non-integer or negative
// value, which callers ignore.
func parseShadowVerifyEvery(getenv func(string) string) (n int, set bool, err error) {
	raw := strings.TrimSpace(getenv(shadowVerifyEveryEnv))
	if raw == "" {
		return 0, false, nil
	}
	n, err = strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, false, fmt.Errorf("%s=%q: want a non-negative integer", shadowVerifyEveryEnv, raw)
	}
	return n, true, nil
}

// applyShadowVerifyEnv applies the env override to s once at startup.
func applyShadowVerifyEnv(s *unfinished.Scanner, getenv func(string) string) {
	n, set, err := parseShadowVerifyEvery(getenv)
	if err != nil {
		log.Warn("ignoring shadow-verify rate override", "err", err)
		return
	}
	if set {
		s.SetShadowVerifyEvery(n)
	}
}
