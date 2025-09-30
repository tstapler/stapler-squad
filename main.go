package main

import (
	"claude-squad/app"
	cmdbridge "claude-squad/cmd"
	"claude-squad/config"
	"claude-squad/daemon"
	"claude-squad/executor"
	"claude-squad/log"
	"claude-squad/server"
	"claude-squad/session"
	"claude-squad/session/git"
	"claude-squad/session/tmux"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
)

var (
	version     = "1.0.12"
	programFlag string
	autoYesFlag bool
	daemonFlag  bool
	webFlag     bool
	rootCmd     = &cobra.Command{
		Use:   "claude-squad",
		Short: "Claude Squad - Manage multiple AI agents like Claude Code, Aider, Codex, and Amp.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
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
				// Disable console output for TUI mode to prevent interfering with BubbleTea
				ConsoleEnabled: false,
				FileEnabled:    true,
				FileLevel:      log.DEBUG,
			}
			log.InitializeWithConfig(daemonFlag, logCfg)
			defer func() {
				log.LogSessionPathsToStderr()
				log.Close()
			}()

			if daemonFlag {
				err := daemon.RunDaemon(cfg)
				log.ErrorLog.Printf("failed to start daemon %v", err)
				return err
			}

			// Web server mode
			if webFlag {
				srv := server.NewServer(":8543")
				log.InfoLog.Printf("Starting web server mode on :8543")
				return srv.Start(ctx)
			}

			// Note: No longer requiring git repository - contextual discovery allows running from anywhere

			// Program flag overrides config
			program := cfg.DefaultProgram
			if programFlag != "" {
				program = programFlag
			}
			// AutoYes flag overrides config
			autoYes := cfg.AutoYes
			if autoYesFlag {
				autoYes = true
			}
			if autoYes {
				defer func() {
					if err := daemon.LaunchDaemon(); err != nil {
						log.ErrorLog.Printf("failed to launch daemon: %v", err)
					}
				}()
			}
			// Kill any daemon that's running.
			if err := daemon.StopDaemon(); err != nil {
				log.ErrorLog.Printf("failed to stop daemon: %v", err)
			}

			return app.Run(ctx, program, autoYes)
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

			state := config.LoadState()
			storage, err := session.NewStorage(state)
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

			// Terminal size detection
			fmt.Println("\n=== Terminal Size Detection ===")
			sizeInfo := app.DetectTerminalSize()

			fmt.Printf("PTY Size:        %dx%d\n", sizeInfo.PTYWidth, sizeInfo.PTYHeight)
			fmt.Printf("Environment:     %dx%d", sizeInfo.EnvWidth, sizeInfo.EnvHeight)
			if sizeInfo.EnvWidth == 0 && sizeInfo.EnvHeight == 0 {
				fmt.Printf(" (not set)")
			}
			fmt.Printf("\n")

			width, height, method := app.GetReliableTerminalSize()
			fmt.Printf("Chosen Size:     %dx%d (method: %s)\n", width, height, method)
			fmt.Printf("Reliability:     %v\n", sizeInfo.IsReliable)

			if len(sizeInfo.Issues) > 0 {
				fmt.Printf("\n⚠️  Detected Issues:\n")
				for _, issue := range sizeInfo.Issues {
					fmt.Printf("  - %s\n", issue)
				}
			} else {
				fmt.Printf("✅ No terminal size issues detected\n")
			}

			// Show environment information that might affect terminal detection
			fmt.Println("\n=== Environment Information ===")
			env_vars := []string{"TERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION", "DESKTOP_SESSION", "XDG_SESSION_TYPE",
				"COLUMNS", "LINES"}
			for _, env := range env_vars {
				if value := os.Getenv(env); value != "" {
					fmt.Printf("%-30s %s\n", env+":", value)
				}
			}

			// Show tiling window manager compatibility info
			fmt.Println("\n=== Tiling Window Manager Compatibility ===")
			fmt.Printf("Detected size: %dx%d (method: %s)\n", width, height, method)
			fmt.Println("")
			fmt.Println("For tiling window managers (like Amethyst):")
			fmt.Println("- Alt screen buffer positions at PTY (0,0) not visible area (0,0)")
			fmt.Println("- Working on automatic detection of actual visible area vs PTY size")
			fmt.Println("- Currently investigating ANSI escape sequences for accurate detection")
			fmt.Println("")
			if width > 100 || height > 50 {
				fmt.Printf("⚠️  Large PTY size detected (%dx%d) - likely indicates PTY/visible area mismatch in tiling WM\n", width, height)
				fmt.Println("   Automatic detection of actual visible area is needed")
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
)

func init() {
	rootCmd.Flags().StringVarP(&programFlag, "program", "p", "",
		"Program to run in new instances (e.g. 'aider --model ollama_chat/gemma3:1b')")
	rootCmd.Flags().BoolVarP(&autoYesFlag, "autoyes", "y", false,
		"[experimental] If enabled, all instances will automatically accept prompts")
	rootCmd.Flags().BoolVar(&daemonFlag, "daemon", false, "Run a program that loads all sessions"+
		" and runs autoyes mode on them.")
	rootCmd.Flags().BoolVar(&webFlag, "web", false, "Run HTTP server with ConnectRPC API on :8543")

	// Hide the daemonFlag as it's only for internal use
	err := rootCmd.Flags().MarkHidden("daemon")
	if err != nil {
		panic(err)
	}

	rootCmd.AddCommand(debugCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(resetCmd)
}

func main() {
	// Set up signal handling for SIGTERM only (not SIGINT/Ctrl+C)
	// BubbleTea handles Ctrl+C (SIGINT) internally for graceful shutdown
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
