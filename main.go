package main

import (
	cmdbridge "claude-squad/cmd"
	"claude-squad/config"
	"claude-squad/daemon"
	"claude-squad/executor"
	"claude-squad/log"
	"claude-squad/profiling"
	"claude-squad/server"
	"claude-squad/session"
	"claude-squad/session/git"
	"claude-squad/session/tmux"
	"claude-squad/telemetry"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var (
	version           = "1.0.12"
	daemonFlag        bool
	testModeFlag      bool
	testDirFlag       string
	discoveryModeFlag string
	discoverExtFlag   bool
	profileFlag       bool
	profilePortFlag   int
	traceFlag         bool
	rootCmd           = &cobra.Command{
		Use:   "claude-squad",
		Short: "Claude Squad - Manage multiple AI agents like Claude Code, Aider, Codex, and Amp (Web Mode)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			// Enable test mode if flag is set
			if testModeFlag {
				testDir := testDirFlag
				if testDir == "" {
					// Use default test directory with PID for isolation
					testDir = fmt.Sprintf("/tmp/claude-squad-test-%d", os.Getpid())
				}
				// Set environment variable for config package to use
				os.Setenv("CLAUDE_SQUAD_TEST_DIR", testDir)
				log.InfoLog.Printf("Test mode enabled: using isolated data directory %s", testDir)
			}

			// Load config first so we can configure logging properly
			cfg := config.LoadConfig()

			// Load discovery config
			discoveryCfg := config.LoadDiscoveryConfig()

			// Apply discovery mode flag overrides
			if discoveryModeFlag != "" {
				mode := config.DiscoveryMode(discoveryModeFlag)
				// Validate the mode
				if mode != config.DiscoveryManagedOnly &&
					mode != config.DiscoveryExternalOnly &&
					mode != config.DiscoveryAll {
					return fmt.Errorf("invalid discovery mode '%s', must be one of: managed-only, external-only, all", discoveryModeFlag)
				}
				discoveryCfg.Mode = mode
				log.InfoLog.Printf("Discovery mode set to: %s (from --discovery-mode flag)", mode)
			}

			// Apply --discover-external shorthand flag
			if discoverExtFlag {
				discoveryCfg.Mode = config.DiscoveryAll
				discoveryCfg.AllowExternalAttach = true
				log.InfoLog.Printf("External discovery enabled (from --discover-external flag)")
			}

			// Convert config to log config
			logCfg := &log.LogConfig{
				LogsEnabled:    true,
				LogsDir:        "", // Use default location
				LogMaxSize:     cfg.LogMaxSize,
				LogMaxFiles:    cfg.LogMaxFiles,
				LogMaxAge:      cfg.LogMaxAge,
				LogCompress:    cfg.LogCompress,
				UseSessionLogs: cfg.UseSessionLogs,
				ConsoleEnabled: false,
				FileEnabled:    true,
				FileLevel:      log.DEBUG,
			}
			log.InitializeWithConfig(daemonFlag, logCfg)
			defer func() {
				log.LogSessionPathsToStderr()
				log.Close()
			}()

			// Start profiling if enabled
			if profileFlag || traceFlag {
				cleanup, err := profiling.StartProfiling(profiling.Config{
					Enabled:      true,
					HTTPPort:     profilePortFlag,
					BlockProfile: true, // Enable block profiling for lock-up detection
					MutexProfile: true, // Enable mutex profiling for lock contention
					TraceEnabled: traceFlag,
					TraceFile:    "", // Use default
				})
				if err != nil {
					return fmt.Errorf("failed to start profiling: %w", err)
				}
				defer cleanup()

				// Monitor goroutines periodically
				if profileFlag {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					go profiling.MonitorGoroutines(ctx, 10*time.Second)
				}
			}

			if daemonFlag {
				err := daemon.RunDaemon(cfg)
				log.ErrorLog.Printf("failed to start daemon %v", err)
				return err
			}

			// Web server mode (default and only mode)
			// Initialize OpenTelemetry for APM (Datadog, etc.)
			telemetryCfg := telemetry.DefaultConfig()
			telemetryProvider, err := telemetry.Initialize(ctx, telemetryCfg)
			if err != nil {
				log.WarningLog.Printf("Failed to initialize telemetry: %v", err)
			} else {
				defer func() {
					shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					if err := telemetryProvider.Shutdown(shutdownCtx); err != nil {
						log.WarningLog.Printf("Failed to shutdown telemetry: %v", err)
					}
				}()
			}

			// Use PORT environment variable if set (for test mode), otherwise default to 8543
			address := "localhost:8543"
			if port := os.Getenv("PORT"); port != "" {
				address = "localhost:" + port
			}
			srv := server.NewServer(address)
			log.InfoLog.Printf("Starting web server on %s", address)
			return srv.Start(ctx)
		},
	}

	resetCmd = &cobra.Command{
		Use:   "reset",
		Short: "Reset all stored instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load config first so we can configure logging properly
			cfg := config.LoadConfig()
			// Convert config to log config
			logCfg := &log.LogConfig{
				LogsEnabled:    true,
				LogsDir:        "", // Use default location
				LogMaxSize:     cfg.LogMaxSize,
				LogMaxFiles:    cfg.LogMaxFiles,
				LogMaxAge:      cfg.LogMaxAge,
				LogCompress:    cfg.LogCompress,
				UseSessionLogs: cfg.UseSessionLogs,
				// Disable console output for CLI commands to keep output clean
				ConsoleEnabled: false,
				FileEnabled:    true,
				FileLevel:      log.DEBUG,
			}
			log.InitializeWithConfig(false, logCfg)
			defer func() {
				log.LogSessionPathsToStderr()
				log.Close()
			}()

			repo, err := session.NewEntRepository()
			if err != nil {
				return fmt.Errorf("failed to initialize repository: %w", err)
			}
			defer repo.Close()
			storage, err := session.NewStorageWithRepository(repo)
			if err != nil {
				return fmt.Errorf("failed to initialize storage: %w", err)
			}
			if err := storage.DeleteAllInstances(); err != nil {
				return fmt.Errorf("failed to reset storage: %w", err)
			}
			fmt.Println("Storage has been reset successfully")

			if err := tmux.CleanupSessions(executor.MakeExecutor()); err != nil {
				return fmt.Errorf("failed to cleanup tmux sessions: %w", err)
			}
			fmt.Println("Tmux sessions have been cleaned up")

			if err := git.CleanupWorktrees(); err != nil {
				return fmt.Errorf("failed to cleanup worktrees: %w", err)
			}
			fmt.Println("Worktrees have been cleaned up")

			// Kill any daemon that's running.
			if err := daemon.StopDaemon(); err != nil {
				return err
			}
			fmt.Println("daemon has been stopped")

			return nil
		},
	}

	debugCmd = &cobra.Command{
		Use:   "debug",
		Short: "Print debug information like config paths",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load config first so we can configure logging properly
			cfg := config.LoadConfig()
			// Convert config to log config
			logCfg := &log.LogConfig{
				LogsEnabled:    true,
				LogsDir:        "", // Use default location
				LogMaxSize:     cfg.LogMaxSize,
				LogMaxFiles:    cfg.LogMaxFiles,
				LogMaxAge:      cfg.LogMaxAge,
				LogCompress:    cfg.LogCompress,
				UseSessionLogs: cfg.UseSessionLogs,
				// Disable console output for CLI commands to keep output clean
				ConsoleEnabled: false,
				FileEnabled:    true,
				FileLevel:      log.DEBUG,
			}
			log.InitializeWithConfig(false, logCfg)
			defer func() {
				log.LogSessionPathsToStderr()
				log.Close()
			}()

			configDir, err := config.GetConfigDir()
			if err != nil {
				return fmt.Errorf("failed to get config directory: %w", err)
			}
			configJson, _ := json.MarshalIndent(cfg, "", "  ")

			fmt.Printf("Config: %s\n%s\n", filepath.Join(configDir, config.ConfigFileName), configJson)

			// Check for key binding conflicts
			fmt.Println("\n=== Key Binding Validation ===")
			bridge := cmdbridge.GetGlobalBridge()
			issues := bridge.ValidateSetup()

			if len(issues) > 0 {
				fmt.Printf("❌ Found %d validation issues:\n", len(issues))
				for _, issue := range issues {
					fmt.Printf("  - %s\n", issue)
				}
			} else {
				fmt.Println("✅ No key binding conflicts detected")
			}


			return nil
		},
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version number of claude-squad",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("claude-squad version %s\n", version)
			fmt.Printf("https://github.com/smtg-ai/claude-squad/releases/tag/v%s\n", version)
		},
	}

	testPtyCmd = &cobra.Command{
		Use:   "test-pty",
		Short: "Test PTY initialization and discovery",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize logging
			cfg := config.LoadConfig()
			logCfg := &log.LogConfig{
				LogsEnabled:    true,
				LogsDir:        "",
				LogMaxSize:     cfg.LogMaxSize,
				LogMaxFiles:    cfg.LogMaxFiles,
				LogMaxAge:      cfg.LogMaxAge,
				LogCompress:    cfg.LogCompress,
				UseSessionLogs: cfg.UseSessionLogs,
				ConsoleEnabled: true,
				FileEnabled:    true,
				FileLevel:      log.DEBUG,
			}
			log.InitializeWithConfig(false, logCfg)
			defer log.Close()

			fmt.Println("=== PTY Initialization Test ===")

			// Load existing sessions
			repo, err := session.NewEntRepository()
			if err != nil {
				return fmt.Errorf("failed to initialize repository: %w", err)
			}
			defer repo.Close()
			storage, err := session.NewStorageWithRepository(repo)
			if err != nil {
				return fmt.Errorf("failed to initialize storage: %w", err)
			}

			instances, err := storage.LoadInstances()
			if err != nil {
				return fmt.Errorf("failed to load instances: %w", err)
			}

			fmt.Printf("Found %d sessions\n\n", len(instances))

			// Test PTY initialization for each session
			for _, inst := range instances {
				fmt.Printf("Session: %s\n", inst.Title)
				fmt.Printf("  Status: %v\n", inst.Status)
				fmt.Printf("  Path: %s\n", inst.Path)
				fmt.Printf("  Branch: %s\n", inst.Branch)

				// Test PTY access
				if inst.Started() {
					ptyReader, err := inst.GetPTYReader()
					if err != nil {
						fmt.Printf("  ❌ PTY Error: %v\n", err)
					} else {
						fmt.Printf("  ✅ PTY: FD %d\n", ptyReader.Fd())

						// Try to get PTY path (cross-platform)
						var ptyPath string
						linuxPath := fmt.Sprintf("/proc/self/fd/%d", ptyReader.Fd())
						macosPath := fmt.Sprintf("/dev/fd/%d", ptyReader.Fd())

						ptyPath, err = os.Readlink(linuxPath)
						if err != nil {
							ptyPath, err = os.Readlink(macosPath)
							if err != nil {
								fmt.Printf("  ⚠️  PTY Path: Unable to resolve (error: %v)\n", err)
							} else {
								fmt.Printf("  ✅ PTY Path: %s (macOS)\n", ptyPath)
							}
						} else {
							fmt.Printf("  ✅ PTY Path: %s (Linux)\n", ptyPath)
						}
					}
				} else {
					fmt.Printf("  ⚠️  Session not started\n")
				}
				fmt.Println()
			}

			// Test PTY discovery
			fmt.Println("=== PTY Discovery Test ===")
			discovery := session.NewPTYDiscovery()
			discovery.SetSessions(instances)
			err = discovery.Refresh()
			if err != nil {
				fmt.Printf("❌ Discovery Error: %v\n", err)
			} else {
				connections := discovery.GetConnections()
				fmt.Printf("Discovered %d PTY connections:\n\n", len(connections))
				for i, conn := range connections {
					fmt.Printf("%d. Session: %s\n", i+1, conn.SessionName)
					fmt.Printf("   Path: %s\n", conn.Path)
					fmt.Printf("   PID: %d\n", conn.PID)
					fmt.Printf("   Command: %s\n", conn.Command)
					fmt.Printf("   Status: %v\n", conn.Status)
					fmt.Println()
				}
			}

			return nil
		},
	}

	listSessionsCmd = &cobra.Command{
		Use:   "list",
		Short: "List all sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize logging
			cfg := config.LoadConfig()
			logCfg := &log.LogConfig{
				LogsEnabled:    true,
				LogsDir:        "",
				LogMaxSize:     cfg.LogMaxSize,
				LogMaxFiles:    cfg.LogMaxFiles,
				LogMaxAge:      cfg.LogMaxAge,
				LogCompress:    cfg.LogCompress,
				UseSessionLogs: cfg.UseSessionLogs,
				ConsoleEnabled: false,
				FileEnabled:    true,
				FileLevel:      log.DEBUG,
			}
			log.InitializeWithConfig(false, logCfg)
			defer log.Close()

			repo, err := session.NewEntRepository()
			if err != nil {
				return fmt.Errorf("failed to initialize repository: %w", err)
			}
			defer repo.Close()
			storage, err := session.NewStorageWithRepository(repo)
			if err != nil {
				return fmt.Errorf("failed to initialize storage: %w", err)
			}

			instances, err := storage.LoadInstances()
			if err != nil {
				return fmt.Errorf("failed to load instances: %w", err)
			}

			if len(instances) == 0 {
				fmt.Println("No sessions found")
				return nil
			}

			fmt.Printf("Found %d sessions:\n\n", len(instances))
			for i, inst := range instances {
				fmt.Printf("%d. %s\n", i+1, inst.Title)
				fmt.Printf("   Status: %v\n", inst.Status)
				fmt.Printf("   Path: %s\n", inst.Path)
				if inst.Branch != "" {
					fmt.Printf("   Branch: %s\n", inst.Branch)
				}
				if inst.Started() {
					fmt.Printf("   Started: Yes\n")
				} else {
					fmt.Printf("   Started: No\n")
				}
				fmt.Println()
			}

			return nil
		},
	}
)

func init() {
	rootCmd.Flags().BoolVar(&daemonFlag, "daemon", false, "Run a program that loads all sessions"+
		" and runs autoyes mode on them.")
	rootCmd.Flags().BoolVar(&testModeFlag, "test-mode", false, "Run in test mode with isolated data directory")
	rootCmd.Flags().StringVar(&testDirFlag, "test-dir", "", "Custom test data directory (defaults to /tmp/claude-squad-test-<PID>)")

	// Discovery mode flags
	rootCmd.Flags().StringVar(&discoveryModeFlag, "discovery-mode", "",
		"Instance discovery mode: managed-only, external-only, or all (default: managed-only)")
	rootCmd.Flags().BoolVar(&discoverExtFlag, "discover-external", false,
		"Enable external instance discovery (shorthand for --discovery-mode=all with attach enabled)")

	// Profiling flags
	rootCmd.Flags().BoolVar(&profileFlag, "profile", false, "Enable runtime profiling (HTTP server + goroutine monitoring)")
	rootCmd.Flags().IntVar(&profilePortFlag, "profile-port", 6060, "Port for pprof HTTP server (default: 6060)")
	rootCmd.Flags().BoolVar(&traceFlag, "trace", false, "Enable execution tracing to /tmp/claude-squad-trace-<PID>.out")

	// Hide the daemonFlag as it's only for internal use
	err := rootCmd.Flags().MarkHidden("daemon")
	if err != nil {
		panic(err)
	}

	rootCmd.AddCommand(debugCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(resetCmd)
	rootCmd.AddCommand(testPtyCmd)
	rootCmd.AddCommand(listSessionsCmd)
}

func main() {
	// Set up signal handling for SIGTERM only (not SIGINT/Ctrl+C)
	// We only intercept SIGTERM for forced termination (e.g., systemd, docker)
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM) // Only SIGTERM, not os.Interrupt

	go func() {
		<-c
		log.InfoLog.Printf("Received SIGTERM, forcing exit")
		log.LogSessionPathsToStderr()
		os.Exit(1)
	}()

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
	}
}
