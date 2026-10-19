package claudehooks

import "strconv"

// block.go bounds how long the AskUserQuestion hook may wait for a server answer
// (ADR-005 rung 1, Task 3.4c). The default is 0: never block, so the terminal
// stays an answer path.

const (
	// HookBlockMsCeiling is the longest wait allowed: the longest block spike S2
	// measured (300 s) with the native dialog still appearing afterwards.
	HookBlockMsCeiling = 300000
	// HookBlockMsEnv names the variable that opts a hook into waiting.
	HookBlockMsEnv = "STAPLER_SQUAD_HOOK_BLOCK_MS"
	// hookTimeoutMaxSeconds is Claude's own default hook timeout (600 s).
	hookTimeoutMaxSeconds = 600
	// hookTimeoutMarginSeconds keeps the hook's own empty-exit cap ahead of Claude's timeout.
	hookTimeoutMarginSeconds = 10
)

// CapHookBlockMs clamps a requested wait to [0, HookBlockMsCeiling].
func CapHookBlockMs(ms int) int {
	return min(max(ms, 0), HookBlockMsCeiling)
}

// HookBlockMsFromEnv reads the wait from the environment, capped; an absent or
// malformed value is 0 (do not wait).
func HookBlockMsFromEnv(lookup func(string) (string, bool)) int {
	v, ok := lookup(HookBlockMsEnv)
	if !ok {
		return 0
	}
	ms, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return CapHookBlockMs(ms)
}

// HookTimeoutSeconds is the `timeout` the injected hook must declare for a wait
// of blockMs: strictly larger than the wait, so the hook's own cap fires first,
// and below Claude's 600 s default.
func HookTimeoutSeconds(blockMs int) int {
	secs := (CapHookBlockMs(blockMs)+999)/1000 + hookTimeoutMarginSeconds
	return min(secs, hookTimeoutMaxSeconds-1)
}
