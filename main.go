package main

import (
	cmdbridge "claude-squad/cmd"
	"claude-squad/config"
	"claude-squad/daemon"
	"claude-squad/executor"
	"claude-squad/log"
	"claude-squad/profiling"
	"claude-squad/server"
	serverauth "claude-squad/server/auth"
	"claude-squad/server/middleware"
	"claude-squad/session"
	"claude-squad/session/git"
	"claude-squad/session/tmux"
	"claude-squad/telemetry"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
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
	listenAddrFlag    string
	remoteAccessFlag  bool
	rpIDFlag          string
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

			// Determine listen address: flag > config > PORT env > default
			address := cfg.ListenAddress
			if address == "" {
				address = "localhost:8543"
			}
			// PORT env var overrides for test mode
			if port := os.Getenv("PORT"); port != "" {
				address = "localhost:" + port
			}
			// --remote-access flag: bind to all interfaces
			if remoteAccessFlag {
				_, port, err := net.SplitHostPort(address)
				if err != nil {
					port = "8543"
				}
				address = "0.0.0.0:" + port
			}
			// --listen flag: explicit override (highest priority)
			if listenAddrFlag != "" {
				address = listenAddrFlag
			}

			// Warn when binding to non-localhost
			host, _, _ := net.SplitHostPort(address)
			if host != "localhost" && host != "127.0.0.1" && host != "::1" {
				log.WarningLog.Printf("WARNING: Binding to non-localhost address %s. Ensure firewall rules are configured.", address)
				fmt.Fprintf(os.Stderr, "\nWARNING: claude-squad is listening on %s (all interfaces).\nEnsure this is intentional and your network is secured.\n\n", address)
			}

			// --rp-id flag overrides config
			if rpIDFlag != "" {
				cfg.PasskeyRPID = rpIDFlag
			}

			srv := server.NewServer(address)

			// Set up passkey auth when remote access is enabled or explicitly configured.
			isRemote := host != "localhost" && host != "127.0.0.1" && host != "::1"
			if isRemote || cfg.PasskeyEnabled {
				if err := setupPasskeyAuth(srv, address, cfg); err != nil {
					return fmt.Errorf("setup passkey auth: %w", err)
				}
			}

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
			state := config.LoadState()
			storage, err := session.NewStorage(state)
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

			state := config.LoadState()
			storage, err := session.NewStorage(state)
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

	// Remote access and passkey flags
	rootCmd.Flags().StringVar(&listenAddrFlag, "listen", "",
		"Address to listen on (e.g. '0.0.0.0:8543'). Overrides config listen_address.")
	rootCmd.Flags().BoolVar(&remoteAccessFlag, "remote-access", false,
		"Enable remote access by binding to 0.0.0.0 (shorthand for --listen 0.0.0.0:<port>)")
	rootCmd.Flags().StringVar(&rpIDFlag, "rp-id", "",
		"WebAuthn Relying Party ID (your LAN IP or hostname, e.g. '192.168.1.42'). "+
			"Required for passkey auth when using remote access.")

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

// setupPasskeyAuth initialises TLS + WebAuthn and wires them into srv.
func setupPasskeyAuth(srv *server.Server, address string, cfg *config.Config) error {
	// Determine the host for the TLS cert SANs and WebAuthn rpID.
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		host = address
		port = "8543"
	}
	if host == "0.0.0.0" || host == "::" {
		// Listening on all interfaces; use localhost as fallback SAN.
		// The user must also set cfg.PasskeyRPID to their LAN IP/hostname.
		host = "localhost"
	}

	// Build SAN list for the TLS cert.
	sans := []string{host, "localhost", "127.0.0.1"}

	// Generate / reuse TLS certs.
	tlsPaths, err := server.EnsureTLSCerts(sans)
	if err != nil {
		return fmt.Errorf("ensure TLS certs: %w", err)
	}

	tlsCfg, err := server.LoadTLSConfig(tlsPaths.CertFile, tlsPaths.KeyFile)
	if err != nil {
		return fmt.Errorf("load TLS config: %w", err)
	}
	srv.SetupTLS(tlsCfg)

	// Determine rpID: config > flag > inferred host.
	rpID := cfg.PasskeyRPID
	if rpID == "" {
		rpID = host
	}
	if rpID == "0.0.0.0" || rpID == "::" {
		rpID = "localhost"
	}

	origin := fmt.Sprintf("https://%s:%s", rpID, port)
	origins := []string{origin, "https://localhost:" + port}
	// Deduplicate
	seen := map[string]bool{}
	unique := origins[:0]
	for _, o := range origins {
		if !seen[o] {
			seen[o] = true
			unique = append(unique, o)
		}
	}

	// Initialise auth subsystem.
	store, err := serverauth.NewCredentialStore()
	if err != nil {
		return fmt.Errorf("create credential store: %w", err)
	}

	sessions := serverauth.NewSessionManager()

	waHandler, err := serverauth.NewHandler(rpID, unique, store, sessions)
	if err != nil {
		return fmt.Errorf("create webauthn handler: %w", err)
	}

	setupMgr := serverauth.NewSetupManager()

	// Register auth routes on the server's mux.
	serverauth.RegisterRoutes(srv.Mux(), waHandler, sessions, store, setupMgr, tlsPaths.CAFile)

	// Apply auth middleware (protects all non-exempt routes).
	srv.SetupAuth(middleware.Auth(sessions))

	// Bootstrap: if no passkeys are registered, generate a setup token and
	// print it + a QR code to stderr so the operator can enroll the first device.
	if !store.HasCredentials() {
		token, tokenErr := setupMgr.Init()
		if tokenErr != nil {
			log.WarningLog.Printf("Failed to generate setup token: %v", tokenErr)
		} else {
			setupURL := fmt.Sprintf("https://%s:%s/login?setup_token=%s", rpID, port, token)
			fmt.Fprintf(os.Stderr, "\n╔══════════════════════════════════════════════════════╗\n")
			fmt.Fprintf(os.Stderr, "║  PASSKEY SETUP REQUIRED                              ║\n")
			fmt.Fprintf(os.Stderr, "╠══════════════════════════════════════════════════════╣\n")
			fmt.Fprintf(os.Stderr, "║  No passkeys registered. Scan the QR code or visit: ║\n")
			fmt.Fprintf(os.Stderr, "║  %-52s ║\n", setupURL)
			fmt.Fprintf(os.Stderr, "║  CA cert for phone trust: %s/auth/ca.pem%-10s ║\n",
				fmt.Sprintf("https://%s:%s", rpID, port), "")
			fmt.Fprintf(os.Stderr, "╚══════════════════════════════════════════════════════╝\n")
			if qrErr := serverauth.PrintQRToTerminal(setupURL); qrErr != nil {
				log.WarningLog.Printf("QR print failed: %v", qrErr)
			}
		}
	}

	// Warn if rpID looks misconfigured.
	if strings.HasPrefix(rpID, "0.0.0.0") || rpID == "::" {
		log.WarningLog.Printf("WARNING: PasskeyRPID=%q is a wildcard address. "+
			"Passkeys will not work. Set --rp-id to your actual LAN IP or hostname.", rpID)
	}

	log.InfoLog.Printf("auth: passkey auth enabled – rpID=%s origin=%s", rpID, origin)
	log.InfoLog.Printf("auth: TLS CA cert: %s", tlsPaths.CAFile)
	return nil
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
