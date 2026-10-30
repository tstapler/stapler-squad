package dbmaint

import (
	"os"
	"strconv"
	"time"
)

// Environment overrides for ConfigFromEnv.
const (
	EnvDisable      = "STAPLER_SQUAD_DB_MAINT_DISABLE"    // "1"/"true" turns the loop off
	EnvInterval     = "STAPLER_SQUAD_DB_MAINT_INTERVAL"   // Go duration, e.g. "15m"
	EnvFreeRatio    = "STAPLER_SQUAD_DB_MAINT_FREE_RATIO" // 0..1
	EnvMinFreePages = "STAPLER_SQUAD_DB_MAINT_MIN_FREE_PAGES"
	EnvChunkPages   = "STAPLER_SQUAD_DB_MAINT_CHUNK_PAGES"
	EnvChunkPause   = "STAPLER_SQUAD_DB_MAINT_CHUNK_PAUSE" // Go duration, e.g. "250ms"
)

// ConfigFromEnv returns the defaults overridden by the Env* variables. Invalid
// values are ignored.
func ConfigFromEnv() Config { return configFromLookup(os.Getenv) }

func configFromLookup(getenv func(string) string) Config {
	var c Config
	if v, err := strconv.ParseBool(getenv(EnvDisable)); err == nil {
		c.Disabled = v
	}
	if d, err := time.ParseDuration(getenv(EnvInterval)); err == nil {
		c.Interval = d
	}
	if f, err := strconv.ParseFloat(getenv(EnvFreeRatio), 64); err == nil {
		c.FreeRatio = f
	}
	if n, err := strconv.ParseInt(getenv(EnvMinFreePages), 10, 64); err == nil {
		c.MinFreePages = n
	}
	if n, err := strconv.ParseInt(getenv(EnvChunkPages), 10, 64); err == nil {
		c.ChunkPages = n
	}
	if d, err := time.ParseDuration(getenv(EnvChunkPause)); err == nil {
		c.ChunkPause = d
	}
	return c.WithDefaults()
}
