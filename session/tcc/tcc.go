// Package tcc classifies filesystem paths that macOS TCC (Transparency,
// Consent and Control) guards behind a user consent prompt.
//
// Classification is heuristic and does filesystem I/O, including symlink
// resolution, stat and statfs; these operations can block on unhealthy storage.
// No public API reports a grant's state. Classify estimates whether access may
// need consent, not whether an operation will prompt or succeed.
package tcc

import (
	"os"
	"path/filepath"
	"strings"
)

// Kind is the TCC service a protected path belongs to.
type Kind int

const (
	// None means the path is not TCC-protected.
	None Kind = iota
	// FilesAndFolders covers per-folder grants (Documents, removable and network volumes, iCloud Drive).
	FilesAndFolders
	// FullDiskAccess covers data that only a blanket Full Disk Access grant unlocks.
	FullDiskAccess
)

// System Settings deep links for each service.
const (
	SettingsURLFilesAndFolders = "x-apple.systempreferences:com.apple.preference.security?Privacy_FilesAndFolders"
	SettingsURLFullDiskAccess  = "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles"
)

func (k Kind) String() string {
	switch k {
	case FilesAndFolders:
		return "FilesAndFolders"
	case FullDiskAccess:
		return "FullDiskAccess"
	default:
		return "None"
	}
}

// SettingsURL returns the System Settings pane for k, or "" for None.
func (k Kind) SettingsURL() string {
	switch k {
	case FilesAndFolders:
		return SettingsURLFilesAndFolders
	case FullDiskAccess:
		return SettingsURLFullDiskAccess
	default:
		return ""
	}
}

// Result is the classification of one path. Root is the protected root (for
// example ~/Documents), so callers can key one consent decision per root
// instead of per file.
type Result struct {
	Protected bool
	Root      string
	Service   Kind
}

// Explanation is a short human-readable reason macOS may prompt for this path.
func (r Result) Explanation() string {
	switch r.Service {
	case FilesAndFolders:
		return "macOS asks for permission before an app reads " + r.Root +
			". Allow it under Privacy & Security > Files & Folders."
	case FullDiskAccess:
		return "macOS only lets an app read " + r.Root +
			" with Full Disk Access. Allow it under Privacy & Security > Full Disk Access."
	default:
		return ""
	}
}

// FSInfo is the filesystem type name (statfs f_fstypename) and mount point
// (f_mntonname) of the filesystem holding a path.
type FSInfo struct {
	Type    string
	MountOn string
}

// Classifier holds the environment lookups so tests can inject them.
type Classifier struct {
	// Home is the user's home directory.
	Home string
	// FS reports the filesystem info for an existing path.
	FS func(path string) (FSInfo, error)
	// Device reports the device ID of an existing path.
	Device func(path string) (uint64, error)
	// Exists reports whether a path exists; nil means os.Lstat. Injected so
	// tests do not depend on which directories the host has.
	Exists func(path string) bool
}

// Home-relative roots in canonical spelling. matchRoot always compares
// case-insensitively, assuming the default case-insensitive APFS; a
// case-sensitive volume would over-match (safe: only extra consent prompts).
var (
	homeFilesAndFolders = []string{
		"Documents", "Desktop", "Downloads", "Library/Mobile Documents",
	}
	homeFullDiskAccess = []string{
		"Library/Biome", "Library/Application Support/Knowledge",
		"Library/News", "Library/Daemon Containers",
		"Library/Group Containers/group.com.apple.notes",
		"Library/Mail", "Library/Messages", "Library/Safari", "Library/Cookies",
		"Library/Calendars", "Library/Suggestions", "Library/HomeKit",
		"Library/IdentityServices", "Library/Metadata/CoreSpotlight",
		"Library/Application Support/AddressBook",
		"Library/Application Support/CallHistoryDB",
		"Library/Application Support/com.apple.TCC",
		"Library/Containers/com.apple.Safari",
		"Library/Group Containers/group.com.apple.Safari.SandboxBroker",
	}
	networkFSTypes = map[string]bool{"smbfs": true, "nfs": true, "afpfs": true, "webdav": true}
)

const volumesDir = "/Volumes"

// Classify reports whether path is TCC-protected. Symlinks are resolved first;
// a path that does not exist is classified by its nearest existing ancestor
// plus the remainder, so a not-yet-created file under ~/Documents is protected.
func (c Classifier) Classify(path string) Result {
	if !filepath.IsAbs(path) {
		return Result{}
	}
	p := resolve(path)
	home := resolve(c.Home)

	if c.Home != "" {
		if r, ok := classifyUnderHome(p, home); ok {
			return r
		}
	}
	if r, ok := c.classifyVolume(p, home); ok {
		return r
	}
	return c.classifyNetwork(p)
}

func classifyUnderHome(p, home string) (Result, bool) {
	rel, ok := relUnder(p, home)
	if !ok {
		return Result{}, false
	}
	// FDA first: its roots are deeper than, and may sit inside, a Files-and-Folders root.
	if root, ok := matchRoot(rel, homeFullDiskAccess); ok {
		return Result{Protected: true, Root: filepath.Join(home, root), Service: FullDiskAccess}, true
	}
	if root, ok := matchRoot(rel, homeFilesAndFolders); ok {
		return Result{Protected: true, Root: filepath.Join(home, root), Service: FilesAndFolders}, true
	}
	return Result{}, false
}

// classifyVolume flags /Volumes/<name> mounts on a different device than the
// Data volume (proxied by home, which always lives on it). The boot volume
// appears as a /Volumes symlink and is already resolved away by this point.
func (c Classifier) classifyVolume(p, home string) (Result, bool) {
	rel, ok := relUnder(p, volumesDir)
	if !ok || rel == "." || c.Device == nil {
		return Result{}, false
	}
	root := filepath.Join(volumesDir, strings.SplitN(rel, string(filepath.Separator), 2)[0])
	volDev, err := c.Device(c.existingAncestor(root))
	if err != nil {
		return Result{}, false
	}
	dataDev, err := c.Device(c.existingAncestor(home))
	if err != nil || volDev == dataDev {
		return Result{}, false
	}
	return Result{Protected: true, Root: root, Service: FilesAndFolders}, true
}

func (c Classifier) classifyNetwork(p string) Result {
	if c.FS == nil {
		return Result{}
	}
	info, err := c.FS(c.existingAncestor(p))
	if err != nil || !networkFSTypes[strings.ToLower(info.Type)] {
		return Result{}
	}
	root := info.MountOn
	if root == "" {
		root = p
	}
	return Result{Protected: true, Root: root, Service: FilesAndFolders}
}

// matchRoot returns the canonical root from roots that rel is equal to or
// inside, comparing path components case-insensitively.
func matchRoot(rel string, roots []string) (string, bool) {
	relParts := strings.Split(rel, string(filepath.Separator))
	for _, root := range roots {
		rootParts := strings.Split(root, "/")
		if len(relParts) < len(rootParts) {
			continue
		}
		match := true
		for i, part := range rootParts {
			if !strings.EqualFold(relParts[i], part) {
				match = false
				break
			}
		}
		if match {
			return root, true
		}
	}
	return "", false
}

// relUnder returns p relative to base when p is base or inside it.
func relUnder(p, base string) (string, bool) {
	rel, err := filepath.Rel(base, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// resolve cleans p and resolves symlinks in its longest existing prefix,
// appending the missing remainder unchanged.
func resolve(p string) string {
	p = filepath.Clean(p)
	var rest []string
	for cur := p; ; cur = filepath.Dir(cur) {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			for i := len(rest) - 1; i >= 0; i-- {
				real = filepath.Join(real, rest[i])
			}
			return real
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p
		}
		rest = append(rest, filepath.Base(cur))
	}
}

// existingAncestor returns the nearest ancestor of p (or p itself) that exists,
// so stat-style lookups work for paths not yet created.
func (c Classifier) existingAncestor(p string) string {
	exists := c.Exists
	if exists == nil {
		exists = func(q string) bool { _, err := os.Lstat(q); return err == nil }
	}
	for cur := p; ; cur = filepath.Dir(cur) {
		if exists(cur) || filepath.Dir(cur) == cur {
			return cur
		}
	}
}
