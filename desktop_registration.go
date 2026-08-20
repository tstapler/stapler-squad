package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// linuxDesktopFileName is the .desktop file's basename — also the value
// `xdg-mime query default x-scheme-handler/ssq` reports once registered, so
// registerLinuxScheme compares against this exact string to detect an
// existing registration.
const linuxDesktopFileName = "stapler-squad.desktop"

// desktopFileContent renders the .desktop file that registers execCommand
// (typically the absolute path to the stapler-squad binary, or just
// "stapler-squad" if it's on PATH) as the OS handler for the ssq:// URL
// scheme. See Story 4.2 in
// project_plans/backlog-deep-linking/implementation/plan.md.
func desktopFileContent(execCommand string) string {
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Stapler Squad
Comment=Open ssq:// deep links in Stapler Squad
Exec=%s --open-url %%u
Terminal=false
NoDisplay=true
MimeType=x-scheme-handler/ssq;
`, execCommand)
}

// xdgMimeQueryFunc, xdgMimeDefaultFunc, and updateDesktopDatabaseFunc shell
// out to the xdg-utils CLI tools. Package vars so tests can stub them
// without a real Linux desktop environment (xdg-mime/update-desktop-database
// aren't installed in most CI/dev containers).
var xdgMimeQueryFunc = func(ctx context.Context) (string, error) {
	cmd := safeexec.CommandContext(ctx, "xdg-mime", "query", "default", "x-scheme-handler/ssq")
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

var xdgMimeDefaultFunc = func(ctx context.Context) error {
	cmd := safeexec.CommandContext(ctx, "xdg-mime", "default", linuxDesktopFileName, "x-scheme-handler/ssq")
	return cmd.Run()
}

var updateDesktopDatabaseFunc = func(ctx context.Context, desktopDir string) error {
	cmd := safeexec.CommandContext(ctx, "update-desktop-database", desktopDir)
	return cmd.Run()
}

// registerLinuxScheme writes the .desktop file into desktopDir (typically
// ~/.local/share/applications) and, unless x-scheme-handler/ssq is already
// mapped to linuxDesktopFileName, registers it as the default handler via
// xdg-mime and refreshes the desktop database. Safe to call on every
// `make install-service` run: the .desktop file is always rewritten (cheap,
// idempotent), but the xdg-mime/update-desktop-database calls are skipped
// once registration is already in place, so re-running never duplicates
// registry entries.
func registerLinuxScheme(ctx context.Context, desktopDir, execCommand string) error {
	if err := os.MkdirAll(desktopDir, 0o755); err != nil {
		return fmt.Errorf("register-linux-scheme: create %s: %w", desktopDir, err)
	}
	desktopFilePath := filepath.Join(desktopDir, linuxDesktopFileName)
	if err := os.WriteFile(desktopFilePath, []byte(desktopFileContent(execCommand)), 0o644); err != nil {
		return fmt.Errorf("register-linux-scheme: write %s: %w", desktopFilePath, err)
	}

	if current, err := xdgMimeQueryFunc(ctx); err == nil && current == linuxDesktopFileName {
		// Already the registered default — skip re-registration.
		return nil
	}

	if err := xdgMimeDefaultFunc(ctx); err != nil {
		return fmt.Errorf("register-linux-scheme: xdg-mime default: %w", err)
	}
	if err := updateDesktopDatabaseFunc(ctx, desktopDir); err != nil {
		return fmt.Errorf("register-linux-scheme: update-desktop-database: %w", err)
	}
	return nil
}
