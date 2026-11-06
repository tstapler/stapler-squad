package services

import (
	"context"
	"os"
	"path/filepath"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// fileSource names one resolved file well enough for a background pass to reopen
// it after the request's root is closed, plus the validator the pass must match.
type fileSource struct {
	Dir      string
	Resolved ResolvedPath
	Key      analysisKey
}

func newFileSource(rr *opsRequest, info os.FileInfo) fileSource {
	return fileSource{
		Dir:      rr.Dir,
		Resolved: rr.Resolved,
		Key: analysisKey{
			Path:    filepath.Join(rr.Dir, string(rr.Resolved)),
			Size:    info.Size(),
			MtimeNs: info.ModTime().UnixNano(),
			Inode:   fileInode(info),
		},
	}
}

// analysisRunner reopens the file and analyzes it, reporting errAnalysisStale if
// the size or mtime moved off the validator while it ran.
func (o *FileOps) analysisRunnerFor(src fileSource) analysisRunner {
	return func(ctx context.Context) (*Analysis, error) {
		root, err := o.backend.OpenRoot(src.Dir)
		if err != nil {
			return nil, err
		}
		defer func() { _ = root.Close() }()
		f, err := root.Open(src.Resolved)
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		a, err := Analyze(ctx, f)
		if err != nil {
			return nil, err
		}
		after, err := root.Lstat(src.Resolved)
		if err != nil {
			return nil, err
		}
		k := src.Key
		if a.Size != k.Size || after.Size() != k.Size || after.ModTime().UnixNano() != k.MtimeNs {
			return nil, errAnalysisStale
		}
		return a, nil
	}
}

// analysisEligible reports whether range-read analysis applies to a file of size.
func (o *FileOps) analysisEligible(ctx context.Context, size int64) bool {
	return o.policy.Flags.RangeRead(ctx) && size <= rangeReadCeilingBytes
}

// startAnalysis starts (or joins) the cache-owned background pass for fr and
// returns its current state and result.
func (o *FileOps) startAnalysis(fr *fileRead) (sessionv1.FileAnalysisState, *Analysis) {
	return o.analysis.Start(fr.Src.Key, o.analysisRunnerFor(fr.Src))
}

// fillAnalysis adds the analysis state, and the hashed token once ready, to a
// Stat response. It is a no-op while files:range-read is off or above the ceiling.
func (o *FileOps) fillAnalysis(ctx context.Context, resp *sessionv1.StatFileResponse, fr *fileRead, wantSHA bool) {
	if !o.analysisEligible(ctx, fr.Size) {
		return
	}
	state, a := o.startAnalysis(fr)
	resp.Analysis = state
	if wantSHA && state == analysisStateReady && fr.Size <= rangeHashCapBytes {
		k := fr.Src.Key
		resp.VersionToken = FileVersionToken{Size: k.Size, MtimeNs: k.MtimeNs, SHA256: a.SHA256}.String()
	}
}
