package tcc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeClassifier(t *testing.T, home string, devs map[string]uint64, fs map[string]FSInfo) Classifier {
	t.Helper()
	return Classifier{
		Home: home,
		Device: func(p string) (uint64, error) {
			if d, ok := devs[p]; ok {
				return d, nil
			}
			return 0, errors.New("no device")
		},
		// Only "/", /Volumes and home exist, regardless of the host.
		Exists: func(p string) bool { return p == "/" || p == volumesDir || p == home },
		FS: func(p string) (FSInfo, error) {
			if i, ok := fs[p]; ok {
				return i, nil
			}
			return FSInfo{Type: "apfs", MountOn: "/"}, nil
		},
	}
}

func TestClassify(t *testing.T) {
	home := t.TempDir()
	home, _ = filepath.EvalSymlinks(home)
	vol := volumesDir + "/USB"
	fsMap := map[string]FSInfo{}
	c := fakeClassifier(t, home, nil, fsMap)

	tests := []struct {
		name string
		path string
		want Result
	}{
		{"documents root", home + "/Documents", Result{true, home + "/Documents", FilesAndFolders}},
		{"nested documents", home + "/Documents/a/b.txt", Result{true, home + "/Documents", FilesAndFolders}},
		{"case-insensitive", home + "/desktop/x", Result{true, home + "/Desktop", FilesAndFolders}},
		{"downloads", home + "/Downloads/x.zip", Result{true, home + "/Downloads", FilesAndFolders}},
		{"icloud drive", home + "/Library/Mobile Documents/com~apple~CloudDocs/f", Result{true, home + "/Library/Mobile Documents", FilesAndFolders}},
		{"mail is FDA", home + "/Library/Mail/V10", Result{true, home + "/Library/Mail", FullDiskAccess}},
		{"safari is FDA", home + "/Library/Safari", Result{true, home + "/Library/Safari", FullDiskAccess}},
		{"nested FDA root", home + "/Library/Application Support/AddressBook/x", Result{true, home + "/Library/Application Support/AddressBook", FullDiskAccess}},
		{"biome is FDA", home + "/Library/Biome/streams", Result{true, home + "/Library/Biome", FullDiskAccess}},
		{"knowledge is FDA", home + "/Library/Application Support/Knowledge/k.db", Result{true, home + "/Library/Application Support/Knowledge", FullDiskAccess}},
		{"news is FDA", home + "/Library/News/x", Result{true, home + "/Library/News", FullDiskAccess}},
		{"daemon containers is FDA", home + "/Library/Daemon Containers/x", Result{true, home + "/Library/Daemon Containers", FullDiskAccess}},
		{"notes group container is FDA", home + "/Library/Group Containers/group.com.apple.notes/db", Result{true, home + "/Library/Group Containers/group.com.apple.notes", FullDiskAccess}},
		{"other group container", home + "/Library/Group Containers/group.com.other/x", Result{}},
		{"plain library", home + "/Library/Caches/x", Result{}},
		{"unprotected code dir", home + "/code/repo", Result{}},
		{"sibling prefix not matched", home + "/DocumentsOld/x", Result{}},
		{"home itself", home, Result{}},
		{"relative path", "Documents/x", Result{}},
		{"outside home", "/usr/local/bin", Result{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := c.Classify(tt.path); got != tt.want {
				t.Errorf("Classify(%q) = %+v, want %+v", tt.path, got, tt.want)
			}
		})
	}

	t.Run("removable volume", func(t *testing.T) {
		t.Parallel()
		cv := fakeClassifier(t, home, map[string]uint64{home: 1, "/": 99, volumesDir: 99}, fsMap)
		want := Result{true, vol, FilesAndFolders}
		if got := cv.Classify(vol + "/photos/a.jpg"); got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})
	t.Run("same-device volume not protected", func(t *testing.T) {
		t.Parallel()
		cv := fakeClassifier(t, home, map[string]uint64{home: 1, "/": 1, volumesDir: 1}, fsMap)
		if got := cv.Classify(vol + "/x"); got.Protected {
			t.Errorf("got %+v, want unprotected", got)
		}
	})
	t.Run("device lookup error tolerated", func(t *testing.T) {
		t.Parallel()
		cv := fakeClassifier(t, home, map[string]uint64{}, fsMap)
		if got := cv.Classify(vol + "/x"); got.Protected {
			t.Errorf("got %+v, want unprotected", got)
		}
	})
}

func TestClassifyNetworkFS(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	for _, typ := range []string{"smbfs", "nfs", "afpfs", "webdav", "SMBFS"} {
		t.Run(typ, func(t *testing.T) {
			t.Parallel()
			c := fakeClassifier(t, home, nil, map[string]FSInfo{"/": {Type: typ, MountOn: "/mnt/share"}})
			want := Result{true, "/mnt/share", FilesAndFolders}
			if got := c.Classify("/mnt/share/dir/f"); got != want {
				t.Errorf("got %+v, want %+v", got, want)
			}
		})
	}
	t.Run("local fs", func(t *testing.T) {
		t.Parallel()
		c := fakeClassifier(t, home, nil, nil)
		if got := c.Classify("/mnt/local/f"); got.Protected {
			t.Errorf("got %+v, want unprotected", got)
		}
	})
	t.Run("statfs error tolerated", func(t *testing.T) {
		t.Parallel()
		c := Classifier{Home: home, Exists: func(p string) bool { return p == "/" }, FS: func(string) (FSInfo, error) { return FSInfo{}, errors.New("boom") }}
		if got := c.Classify("/mnt/x"); got.Protected {
			t.Errorf("got %+v, want unprotected", got)
		}
	})
}

func TestClassifyResolvesSymlinks(t *testing.T) {
	t.Parallel()
	home, _ := filepath.EvalSymlinks(t.TempDir())
	if err := os.MkdirAll(filepath.Join(home, "Documents", "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(filepath.Join(home, "Documents", "proj"), link); err != nil {
		t.Fatal(err)
	}
	c := fakeClassifier(t, home, nil, nil)

	want := Result{true, filepath.Join(home, "Documents"), FilesAndFolders}
	if got := c.Classify(link); got != want {
		t.Errorf("symlink into Documents: got %+v, want %+v", got, want)
	}
	// Missing leaf beneath the symlink resolves via its existing ancestor.
	if got := c.Classify(filepath.Join(link, "new", "file.go")); got != want {
		t.Errorf("missing path under symlink: got %+v, want %+v", got, want)
	}
}

func TestKindAndExplanation(t *testing.T) {
	t.Parallel()
	if None.SettingsURL() != "" {
		t.Error("None must have no settings URL")
	}
	if FilesAndFolders.SettingsURL() != SettingsURLFilesAndFolders || FullDiskAccess.SettingsURL() != SettingsURLFullDiskAccess {
		t.Error("settings URL mismatch")
	}
	r := Result{Protected: true, Root: "/Users/x/Documents", Service: FilesAndFolders}
	if !strings.Contains(r.Explanation(), "/Users/x/Documents") {
		t.Errorf("explanation should name the root: %q", r.Explanation())
	}
	if (Result{}).Explanation() != "" {
		t.Error("unprotected result has no explanation")
	}
}
