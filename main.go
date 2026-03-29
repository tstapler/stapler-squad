package main

import (
	cmdbridge "github.com/tstapler/stapler-squad/cmd"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/daemon"
	"github.com/tstapler/stapler-squad/executor"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/profiling"
	"github.com/tstapler/stapler-squad/server"
	serverauth "github.com/tstapler/stapler-squad/server/auth"
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/telemetry"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
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
	remotePortFlag    int
	rpIDFlag          string
	rootCmd           = &cobra.Command{
		Use:   "stapler-squad",
		Short: "Stapler Squad - Manage multiple AI agents like Claude Code, Aider, Codex, and Amp (Web Mode)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			// Enable test mode if flag is set
			if testModeFlag {
				testDir := testDirFlag
				if testDir == "" {
					// Use default test directory with PID for isolation
					testDir = fmt.Sprintf("/tmp/stapler-squad-test-%d", os.Getpid())
				}
				// Set environment variable for config package to use
				os.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)
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
			// --listen flag: explicit override (highest priority)
			if listenAddrFlag != "" {
				address = listenAddrFlag
			}

			// Warn when binding to non-localhost
			host, _, _ := net.SplitHostPort(address)
			if host != "localhost" && host != "127.0.0.1" && host != "::1" {
				log.WarningLog.Printf("WARNING: Binding to non-localhost address %s. Ensure firewall rules are configured.", address)
				fmt.Fprintf(os.Stderr, "\nWARNING: stapler-squad is listening on %s (all interfaces).\nEnsure this is intentional and your network is secured.\n\n", address)
			}

			// --rp-id flag overrides config
			if rpIDFlag != "" {
				cfg.PasskeyRPID = rpIDFlag
			}

			// Detect LAN IP for hostname resolution and display
			lanIP, _ := getOutboundIP()
			lanIPStr := "127.0.0.1"
			if lanIP != nil {
				lanIPStr = lanIP.String()
			}
			hostnames := resolveLANHostnames(lanIPStr)

			srv := server.NewServer(address)
			srv.SetHostnames(hostnames)

			localOrigin := fmt.Sprintf("http://%s", address)
			srv.SetOrigins([]string{localOrigin})

			// Start a second HTTPS server with passkey auth for remote access.
			if remoteAccessFlag || cfg.PasskeyEnabled {
				if err := startRemoteAccess(ctx, srv, address, cfg, remotePortFlag); err != nil {
					return fmt.Errorf("start remote access: %w", err)
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
		Short: "Print the version number of stapler-squad",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("stapler-squad version %s\n", version)
			fmt.Printf("https://github.com/tstapler/stapler-squad/releases/tag/v%s\n", version)
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
	rootCmd.Flags().StringVar(&testDirFlag, "test-dir", "", "Custom test data directory (defaults to /tmp/stapler-squad-test-<PID>)")

	// Discovery mode flags
	rootCmd.Flags().StringVar(&discoveryModeFlag, "discovery-mode", "",
		"Instance discovery mode: managed-only, external-only, or all (default: managed-only)")
	rootCmd.Flags().BoolVar(&discoverExtFlag, "discover-external", false,
		"Enable external instance discovery (shorthand for --discovery-mode=all with attach enabled)")

	// Profiling flags
	rootCmd.Flags().BoolVar(&profileFlag, "profile", false, "Enable runtime profiling (HTTP server + goroutine monitoring)")
	rootCmd.Flags().IntVar(&profilePortFlag, "profile-port", 6060, "Port for pprof HTTP server (default: 6060)")
	rootCmd.Flags().BoolVar(&traceFlag, "trace", false, "Enable execution tracing to /tmp/stapler-squad-trace-<PID>.out")

	// Remote access and passkey flags
	rootCmd.Flags().StringVar(&listenAddrFlag, "listen", "",
		"Address to listen on (e.g. '0.0.0.0:8543'). Overrides config listen_address.")
	rootCmd.Flags().BoolVar(&remoteAccessFlag, "remote-access", false,
		"Enable remote access: starts a second HTTPS server with passkey auth on --remote-port (default 8444). "+
			"Local server on localhost remains unchanged.")
	rootCmd.Flags().IntVar(&remotePortFlag, "remote-port", 8444,
		"Port for the remote access HTTPS server (used with --remote-access, default: 8444)")
	rootCmd.Flags().StringVar(&rpIDFlag, "rp-id", "",
		"WebAuthn Relying Party ID override (your LAN IP or hostname, e.g. '192.168.1.42'). "+
			"Defaults to the detected LAN IP.")

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

// resolveLANHostnames returns a list of domain names suitable for use as a WebAuthn rpID
// or TLS SANs. It collects all identifiable hostnames from various sources.
func resolveLANHostnames(lanIPStr string) []string {
	var hostnames []string
	seen := make(map[string]bool)

	add := func(name string) {
		name = strings.TrimSpace(name)
		if name != "" && strings.Contains(name, ".") && !seen[name] {
			hostnames = append(hostnames, name)
			seen[name] = true
		}
	}

	// 1. Reverse DNS — works when the DHCP server registers PTR records.
	if names, err := net.LookupAddr(lanIPStr); err == nil {
		for _, name := range names {
			// PTR records end with a trailing dot; strip it.
			if len(name) > 0 && name[len(name)-1] == '.' {
				name = name[:len(name)-1]
			}
			add(name)
		}
	}

	// 2. Linux-specific: mDNS reverse lookup via avahi-resolve
	if out, err := exec.Command("avahi-resolve", "-a", lanIPStr).Output(); err == nil {
		fields := strings.Fields(string(out))
		if len(fields) >= 2 {
			add(fields[1])
		}
	}

	// 3. Try hostname -f for FQDN
	if out, err := exec.Command("hostname", "-f").Output(); err == nil {
		add(string(out))
	}

	hostname, hostErr := os.Hostname()
	if hostErr == nil && hostname != "" {
		// 4. hostname + search domains
		for _, domain := range getDNSSearchDomains() {
			if domain == "local" {
				continue
			}
			add(hostname + "." + domain)
		}

		// 5. mDNS .local fallback
		add(hostname + ".local")
	}

	return hostnames
}

// getDNSSearchDomains returns the DNS search domains configured on this system
// by parsing /etc/resolv.conf.  Returns nil if the file cannot be read or has
// no search directive.
func getDNSSearchDomains() []string {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	var domains []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "search ") {
			for _, d := range strings.Fields(line)[1:] {
				domains = append(domains, d)
			}
		}
	}
	return domains
}

// getOutboundIP detects the primary LAN IP by consulting the OS routing table.
// No data is sent – this just triggers a routing lookup.
func getOutboundIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP, nil
}

// startRemoteAccess starts a second HTTPS server on all interfaces with passkey
// authentication, while the local server on localhost stays unchanged.
func startRemoteAccess(ctx context.Context, srv *server.Server, localAddr string, cfg *config.Config, remotePort int) error {
	// Detect LAN IP for QR code URLs and TLS cert SANs.
	lanIP, err := getOutboundIP()
	if err != nil {
		log.WarningLog.Printf("Could not detect LAN IP: %v; using localhost", err)
		lanIP = net.ParseIP("127.0.0.1")
	}
	lanIPStr := lanIP.String()

	// Use hostnames already resolved and stored on the server.
	hostnames := srv.GetHostnames()

	remoteAddr := fmt.Sprintf("0.0.0.0:%d", remotePort)

	// Build SAN list for the TLS cert (include localhost, IP, and all hostnames).
	sans := []string{"localhost", "127.0.0.1", lanIPStr}
	sans = append(sans, hostnames...)

	tlsPaths, err := server.EnsureTLSCerts(sans)
	if err != nil {
		return fmt.Errorf("ensure TLS certs: %w", err)
	}

	tlsCfg, err := server.LoadTLSConfig(tlsPaths.CertFile, tlsPaths.KeyFile)
	if err != nil {
		return fmt.Errorf("load TLS config: %w", err)
	}

	rpID := cfg.PasskeyRPID
	if rpID == "" {
		if len(hostnames) > 0 {
			rpID = hostnames[0]
		} else {
			rpID = lanIPStr
		}
	}
	allRPIDs := []string{rpID}
	if len(hostnames) > 1 {
		allRPIDs = append(allRPIDs, hostnames...)
	}

	origins := []string{fmt.Sprintf("https://%s:%d", rpID, remotePort)}
	for _, hn := range hostnames {
		origins = append(origins, fmt.Sprintf("https://%s:%d", hn, remotePort))
	}
	origins = append(origins, fmt.Sprintf("https://localhost:%d", remotePort))
	srv.SetOrigins(append(srv.GetOrigins(), origins...))

	// Initialise auth subsystem.
	store, err := serverauth.NewCredentialStore()
	if err != nil {
		return fmt.Errorf("create credential store: %w", err)
	}

	// Persist auth sessions so the phone stays logged in across server restarts.
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("get config dir: %w", err)
	}
	sessionsPath := filepath.Join(configDir, "auth-sessions.json")
	sessions := serverauth.NewSessionManager(sessionsPath)

	waHandler, err := serverauth.NewHandler(allRPIDs, origins, store, sessions)
	if err != nil {
		return fmt.Errorf("create webauthn handler: %w", err)
	}

	setupMgr := serverauth.NewSetupManager()

	// Register auth routes on the shared mux (accessible via both servers).
	serverauth.RegisterRoutes(srv.Mux(), waHandler, sessions, store, setupMgr, tlsPaths.CAFile)

	// Start the remote HTTPS server with auth middleware applied.
	if err := srv.StartRemote(ctx, remoteAddr, tlsCfg, middleware.Auth(sessions)); err != nil {
		return fmt.Errorf("start remote server: %w", err)
	}

	// Store the HTTPS URL so /api/server-info can expose it to the settings UI.
	srv.SetHTTPSURL(origin)

	// Bootstrap: if no passkeys registered, print setup + CA QR codes.
	if !store.HasCredentials() {
		token, tokenErr := setupMgr.Init()
		if tokenErr != nil {
			log.WarningLog.Printf("Failed to generate setup token: %v", tokenErr)
		} else {
			setupURL := fmt.Sprintf("https://%s:%d/login?setup_token=%s", displayHost, remotePort, token)
			caURL := fmt.Sprintf("https://%s:%d/auth/ca.pem", displayHost, remotePort)

			fmt.Fprintf(os.Stderr, "\n╔══════════════════════════════════════════════════════╗\n")
			fmt.Fprintf(os.Stderr, "║  REMOTE ACCESS ENABLED                               ║\n")
			fmt.Fprintf(os.Stderr, "╠══════════════════════════════════════════════════════╣\n")
			fmt.Fprintf(os.Stderr, "║  Local  (no auth): http://%-26s ║\n", localAddr)
			fmt.Fprintf(os.Stderr, "║  Remote (HTTPS):   https://%s:%-5d               ║\n", displayHost, remotePort)
			fmt.Fprintf(os.Stderr, "╠══════════════════════════════════════════════════════╣\n")
			fmt.Fprintf(os.Stderr, "║  No passkeys registered — scan QR codes below:       ║\n")
			fmt.Fprintf(os.Stderr, "╚══════════════════════════════════════════════════════╝\n")

			fmt.Fprintf(os.Stderr, "\n── QR Code 1: Install CA certificate (trust HTTPS on your phone) ──\n")
			if qrErr := serverauth.PrintQRToTerminal(caURL); qrErr != nil {
				log.WarningLog.Printf("CA QR print failed: %v", qrErr)
			}

			fmt.Fprintf(os.Stderr, "\n── QR Code 2: Register passkey (after installing CA cert) ──\n")
			if qrErr := serverauth.PrintQRToTerminal(setupURL); qrErr != nil {
				log.WarningLog.Printf("Setup QR print failed: %v", qrErr)
			}
		}
	}

	log.InfoLog.Printf("auth: remote access enabled on port %d – rpID=%s host=%s LAN IP=%s", remotePort, rpID, displayHost, lanIPStr)
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
