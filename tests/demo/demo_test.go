package demo

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestRecordDemo starts an isolated server, seeds it with mock sessions, and
// drives Playwright to record a demo video saved to assets/demo.webm.
//
// Skipped unless RECORD_DEMO=1 is set, so it is safe to include in CI.
//
// Usage:
//
//	RECORD_DEMO=1 go test ./tests/demo/... -run TestRecordDemo -v -timeout 120s
//	make demo-video
func TestRecordDemo(t *testing.T) {
	if os.Getenv("RECORD_DEMO") != "1" {
		t.Skip("Set RECORD_DEMO=1 to record demo video")
	}

	srv := StartDemoServer(t)
	defer srv.Stop()

	if err := srv.WaitForHealth(30 * time.Second); err != nil {
		t.Fatalf("server health check failed: %v", err)
	}

	t.Logf("Demo server running at %s", srv.URL())

	outputDir := filepath.Join(projectRoot(), "assets", "demo-recording")
	_ = os.RemoveAll(outputDir)

	e2eDir := filepath.Join(projectRoot(), "tests", "e2e")
	playwrightBin := filepath.Join(e2eDir, "node_modules", ".bin", "playwright")
	cmd := exec.Command(playwrightBin, "test",
		"--config", "playwright.demo.config.ts",
		"--project=chromium",
	)
	cmd.Env = append(os.Environ(),
		"TEST_SERVER_URL="+srv.URL(),
		"PLAYWRIGHT_VIDEO_OUTPUT_DIR="+outputDir,
	)
	cmd.Dir = e2eDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("playwright failed: %v", err)
	}

	if err := installDemoVideo(outputDir); err != nil {
		t.Fatalf("failed to install demo video: %v", err)
	}
	t.Logf("Demo video saved to assets/demo.webm")
}

// installDemoVideo finds the first .webm file in outputDir and copies it to
// assets/demo.webm in the project root.
func installDemoVideo(outputDir string) error {
	var videoPath string
	err := filepath.WalkDir(outputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if filepath.Ext(path) == ".webm" && videoPath == "" {
			videoPath = path
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking output dir: %w", err)
	}
	if videoPath == "" {
		return fmt.Errorf("no .webm file found in %s", outputDir)
	}

	data, err := os.ReadFile(videoPath)
	if err != nil {
		return fmt.Errorf("reading video: %w", err)
	}

	assetsDir := filepath.Join(projectRoot(), "assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return fmt.Errorf("creating assets dir: %w", err)
	}

	dest := filepath.Join(assetsDir, "demo.webm")
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return fmt.Errorf("writing demo video: %w", err)
	}
	return nil
}
