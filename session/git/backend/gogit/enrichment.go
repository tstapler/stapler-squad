package gogit

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Enrichment contains only local facts, without Git config scopes or CLI fallback.
type Enrichment struct {
	GitDir    string
	CommonDir string
	Origin    string
}

// ReadEnrichment supports an explicit worktree root with a .git directory or
// gitdir file and one commondir link. Unsupported layouts fail closed. Guard
// runs before each metadata operation, including every discovered symlink target.
// This is not handle-based confinement against concurrent symlink replacement.
func ReadEnrichment(root string, guard func(string) error) (Enrichment, error) {
	r := enrichmentReader{guard: guard}
	dotgit := filepath.Join(root, ".git")
	path, err := r.resolve(dotgit)
	if err != nil {
		return Enrichment{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return Enrichment{}, err
	}
	gitdir := path
	if !info.IsDir() {
		data, readErr := r.read(path, maxGitfileSize)
		if readErr != nil {
			return Enrichment{}, readErr
		}
		target, parseErr := parseGitfile(path, data)
		if parseErr != nil {
			return Enrichment{}, parseErr
		}
		gitdir, err = r.resolve(target)
		if err != nil {
			return Enrichment{}, err
		}
	}
	common := gitdir
	data, err := r.read(filepath.Join(gitdir, "commondir"), maxGitfileSize)
	if err == nil {
		target := strings.TrimSpace(string(data))
		if target == "" || strings.ContainsAny(target, "\r\n\x00") {
			return Enrichment{}, errors.New("unsupported commondir pointer")
		}
		common, err = r.resolve(resolveLink(gitdir, target))
		if err != nil {
			return Enrichment{}, err
		}
		if common == gitdir {
			return Enrichment{}, errors.New("cyclic commondir pointer")
		}
		_, nestedErr := r.read(filepath.Join(common, "commondir"), maxGitfileSize)
		if nestedErr == nil {
			return Enrichment{}, errors.New("nested commondir is unsupported")
		}
		if !errors.Is(nestedErr, os.ErrNotExist) {
			return Enrichment{}, nestedErr
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Enrichment{}, err
	}
	data, err = r.read(filepath.Join(common, "config"), maxConfigFileSize)
	if err != nil {
		return Enrichment{}, err
	}
	entries, _, err := parseConfigData(data)
	if err != nil {
		return Enrichment{}, err
	}
	return enrichmentConfig(gitdir, common, entries)
}

func enrichmentConfig(gitdir, common string, entries []cfgEntry) (Enrichment, error) {
	out := Enrichment{GitDir: gitdir, CommonDir: common}
	for _, entry := range entries {
		section := strings.ToLower(entry.section)
		if section == "include" || section == "includeif" || section == "extensions" || (section == "core" && strings.EqualFold(entry.name, "worktree")) {
			return Enrichment{}, errors.New("unsupported enrichment config redirect or extension")
		}
		if section == "remote" && entry.sub == "origin" && strings.EqualFold(entry.name, "url") {
			out.Origin = entry.value
		}
	}
	return out, nil
}

func resolveLink(base, target string) string {
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(base, target)
}

type enrichmentReader struct{ guard func(string) error }

func (r enrichmentReader) resolve(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	for hops := 0; hops < 32; hops++ {
		parts := strings.Split(strings.TrimPrefix(filepath.Clean(path), string(filepath.Separator)), string(filepath.Separator))
		current := string(filepath.Separator)
		changed := false
		for i, part := range parts {
			current = filepath.Join(current, part)
			if err := r.guard(current); err != nil {
				return "", err
			}
			info, err := os.Lstat(current)
			if err != nil {
				return "", err
			}
			if info.Mode()&os.ModeSymlink == 0 {
				continue
			}
			target, err := os.Readlink(current)
			if err != nil {
				return "", err
			}
			path = filepath.Join(resolveLink(filepath.Dir(current), target), filepath.Join(parts[i+1:]...))
			changed = true
			break
		}
		if !changed {
			return path, nil
		}
	}
	return "", errors.New("enrichment symlink hop limit exceeded")
}

func (r enrichmentReader) read(path string, limit int) ([]byte, error) {
	canonical, err := r.resolve(path)
	if err != nil {
		return nil, err
	}
	if err := r.guard(canonical); err != nil {
		return nil, err
	}
	info, err := os.Lstat(canonical)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("enrichment metadata is not a regular file")
	}
	f, err := os.Open(canonical)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("enrichment metadata exceeds %d bytes", limit)
	}
	return data, nil
}
