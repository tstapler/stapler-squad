package clihelp

import (
	"bytes"
	"context"
	"io/fs"
	"log/slog"
)

// Native executable magic numbers: ELF, Mach-O (both byte orders, 32/64-bit), fat.
func isNative(head []byte) bool {
	for _, m := range [][]byte{
		{0x7f, 'E', 'L', 'F'},
		{0xcf, 0xfa, 0xed, 0xfe}, {0xce, 0xfa, 0xed, 0xfe},
		{0xfe, 0xed, 0xfa, 0xce}, {0xfe, 0xed, 0xfa, 0xcf},
		{0xca, 0xfe, 0xba, 0xbe},
	} {
		if bytes.HasPrefix(head, m) {
			return true
		}
	}
	return false
}

// execute runs cache lookup -> execution gate -> flight for a located target.
// Without a runner it reports the program as found with no flags.
func (p *Prober) execute(ctx context.Context, path ResolvedPath, opts ProbeOpts) ProbeResult {
	if p.run == nil {
		return ProbeResult{Status: ProbeStatusFoundNoFlags, ResolvedPath: path}
	}
	info, err := p.stat(string(path))
	if err != nil {
		return ProbeResult{Status: ProbeStatusError, ResolvedPath: path}
	}
	key := keyFor(path, info)

	if res, ok := p.cachedFor(key, opts); ok {
		return res
	}
	if !p.mayRun(path, key, opts) {
		return ProbeResult{Status: ProbeStatusNeedsConfirm, ResolvedPath: path}
	}
	if ctx.Err() != nil {
		return ProbeResult{Status: ProbeStatusError, ResolvedPath: path}
	}

	ch := p.flights.DoChan(key.flightKey(), func() (res any, err error) {
		// The flight runs on its own goroutine: a panic here would take the whole server down.
		defer func() {
			if r := recover(); r != nil {
				slog.Error("program_probe flight panicked", "panic", r)
				res = ProbeResult{Status: ProbeStatusError, ResolvedPath: path}
			}
		}()
		return p.flight(context.WithoutCancel(ctx), path, key, opts), nil
	})
	select {
	case r := <-ch:
		return r.Val.(ProbeResult)
	case <-ctx.Done():
		// The flight keeps running and caches its own result for the next caller.
		return ProbeResult{Status: ProbeStatusError, ResolvedPath: path}
	}
}

// cachedFor returns a live cache entry, except that an explicit Check
// (ConfirmExecute) retries a cached TIMEOUT.
func (p *Prober) cachedFor(key cacheKey, opts ProbeOpts) (ProbeResult, bool) {
	res, ok := p.cache.get(key)
	if !ok || (opts.ConfirmExecute && res.Status == ProbeStatusTimeout) {
		return ProbeResult{}, false
	}
	res.CacheHit = true
	return res, true
}

func keyFor(path ResolvedPath, info fs.FileInfo) cacheKey {
	return cacheKey{RealPath: string(path), MtimeNanos: info.ModTime().UnixNano(), Size: info.Size()}
}

// mayRun is the execution gate: scripts and unrecognised files run only after
// an explicit confirmation; resolve-only never executes an unconfirmed target.
// A ConfirmExecute call records consent for this (path, mtime, size).
func (p *Prober) mayRun(path ResolvedPath, key cacheKey, opts ProbeOpts) bool {
	confirmed := p.confirmed.has(key)
	switch {
	case opts.ResolveOnly:
		return confirmed
	case confirmed || p.isNativeFile(path):
		return true
	case opts.ConfirmExecute:
		p.confirmed.add(key)
		return true
	}
	return false
}

func (p *Prober) isNativeFile(path ResolvedPath) bool {
	head, err := p.readHead(string(path))
	return err == nil && isNative(head)
}

// flight is the singleflight body, run by the leader only. It owns one
// semaphore slot for exactly as long as the child may be alive, so cancelled
// callers can never push the process count past maxConcurrentRuns.
func (p *Prober) flight(ctx context.Context, path ResolvedPath, key cacheKey, opts ProbeOpts) ProbeResult {
	// A flight that finished between the caller's cache miss and this call has
	// already stored its result; re-check so that gap never causes a second run.
	if res, ok := p.cachedFor(key, opts); ok {
		return res
	}
	select {
	case p.sem <- struct{}{}:
		defer func() { <-p.sem }()
	default:
		return ProbeResult{Status: ProbeStatusBusy, ResolvedPath: path}
	}
	out, err := p.run(ctx, path, p.limits)
	res := p.classify(path, out, err)
	// A changed mtime/size means the bytes we ran are no longer the ones on disk.
	if info, serr := p.stat(string(path)); serr == nil && keyFor(path, info) == key {
		p.cache.put(key, res)
	}
	return res
}

func (p *Prober) classify(path ResolvedPath, out RunOutput, err error) ProbeResult {
	res := ProbeResult{ResolvedPath: path, Truncated: out.Truncated}
	switch {
	case err != nil:
		return ProbeResult{Status: ProbeStatusError, ResolvedPath: path}
	case out.TimedOut:
		res.Status = ProbeStatusTimeout
	default:
		res.Flags = ParseHelp(string(out.Text))
		res.Status = ProbeStatusFoundNoFlags
		if len(res.Flags) > 0 {
			res.Status = ProbeStatusFoundParsed
		}
	}
	return res
}
