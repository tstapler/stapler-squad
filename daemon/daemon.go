package daemon

import (
	"claude-squad/config"
	"claude-squad/log"
	"claude-squad/session"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// RunDaemon runs the daemon process which iterates over all sessions and runs AutoYes mode on them.
// It's expected that the main process kills the daemon when the main process starts.
func RunDaemon(cfg *config.Config) error {
	// Log initialization is done by the caller
	log.InfoLog.Printf("starting daemon")
	
	// Load state with built-in locking
	state := config.LoadState()
	// Ensure we release locks when done
	defer state.Close()
	
	storage, err := session.NewStorage(state)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	instances, err := storage.LoadInstances()
	if err != nil {
		return fmt.Errorf("failed to load instances: %w", err)
	}
	for _, instance := range instances {
		// Assume AutoYes is true if the daemon is running.
		instance.AutoYes = true
	}

	pollInterval := time.Duration(cfg.DaemonPollInterval) * time.Millisecond
	
	// Setup file watcher for state.json if enabled
	var watcher *fsnotify.Watcher
	if cfg.DetectNewSessions {
		watcher, err = setupStateFileWatcher()
		if err != nil {
			log.ErrorLog.Printf("failed to setup file watcher: %v", err)
			// Continue without file watching
		} else {
			defer watcher.Close()
		}
	}

	// If we get an error for a session, it's likely that we'll keep getting the error. Log every 30 seconds.
	everyN := log.NewEvery(60 * time.Second)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	stopCh := make(chan struct{})
	
	// AutoYes mode goroutine
	go func() {
		defer wg.Done()
		ticker := time.NewTimer(pollInterval)
		for {
			for _, instance := range instances {
				// We only store started instances, but check anyway.
				if instance.Started() && !instance.Paused() {
					if _, hasPrompt := instance.HasUpdated(); hasPrompt {
						instance.TapEnter()
						if err := instance.UpdateDiffStats(); err != nil {
							if everyN.ShouldLog() {
								log.WarningLog.Printf("could not update diff stats for %s: %v", instance.Title, err)
							}
						}
					}
				}
			}

			// Handle stop before ticker.
			select {
			case <-stopCh:
				return
			default:
			}

			<-ticker.C
			ticker.Reset(pollInterval)
		}
	}()
	
	// Session detection goroutine if enabled
	if watcher != nil && cfg.DetectNewSessions {
		wg.Add(1)
		go watchForNewSessions(watcher, &instances, storage, cfg, stopCh, wg)
	}
	
	// Periodic state refresh goroutine
	if cfg.StateRefreshInterval > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(time.Duration(cfg.StateRefreshInterval) * time.Millisecond)
			defer ticker.Stop()
			
			log.InfoLog.Printf("starting state refresh with interval %d ms", cfg.StateRefreshInterval)
			
			for {
				select {
				case <-ticker.C:
					// Refresh state and load any new instances
					if err := detectAndAddNewSessions(&instances, storage); err != nil {
						log.ErrorLog.Printf("failed to refresh state and detect new sessions: %v", err)
					}
				case <-stopCh:
					log.InfoLog.Printf("stopping state refresh")
					return
				}
			}
		}()
	}

	// Notify on SIGINT (Ctrl+C) and SIGTERM. Save instances before
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	log.InfoLog.Printf("received signal %s", sig.String())

	// Stop the goroutines so we don't race.
	close(stopCh)
	wg.Wait()

	if err := storage.SaveInstances(instances); err != nil {
		log.ErrorLog.Printf("failed to save instances when terminating daemon: %v", err)
	}
	return nil
}

// setupStateFileWatcher initializes a file watcher for configuration files
func setupStateFileWatcher() (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}
	
	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}
	
	// Make sure the config directory exists before watching it
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create config directory: %w", err)
		}
	}
	
	// First, watch the directory itself to catch file creation/deletion
	if err := watcher.Add(configDir); err != nil {
		return nil, fmt.Errorf("failed to add config directory to watcher: %w", err)
	}
	
	// Add individual files to watch
	statePath := filepath.Join(configDir, config.StateFileName)
	if _, err := os.Stat(statePath); err == nil {
		if err := watcher.Add(statePath); err != nil {
			log.WarningLog.Printf("failed to add state file to watcher: %v", err)
		} else {
			log.InfoLog.Printf("watching state file: %s", statePath)
		}
	}
	
	// Also watch instances.json if it exists (legacy or alternate storage)
	instancesPath := filepath.Join(configDir, config.InstancesFileName)
	if _, err := os.Stat(instancesPath); err == nil {
		if err := watcher.Add(instancesPath); err != nil {
			log.WarningLog.Printf("failed to add instances file to watcher: %v", err)
		} else {
			log.InfoLog.Printf("watching instances file: %s", instancesPath)
		}
	}
	
	log.InfoLog.Printf("watching config directory for changes: %s", configDir)
	return watcher, nil
}

// watchForNewSessions watches for state.json file changes and updates instances accordingly
func watchForNewSessions(
	watcher *fsnotify.Watcher,
	instances *[]*session.Instance,
	storage *session.Storage,
	cfg *config.Config,
	stopCh <-chan struct{},
	wg *sync.WaitGroup,
) {
	defer wg.Done()
	
	// We add a small debounce to avoid processing the same change multiple times
	var lastProcessTime time.Time
	debounceTime := 500 * time.Millisecond
	
	// Keep track of state.json file status
	configDir, _ := config.GetConfigDir()
	stateFilePath := filepath.Join(configDir, config.StateFileName)

	// Setup polling ticker for regular checks
	pollTicker := time.NewTicker(time.Duration(cfg.SessionDetectionInterval) * time.Millisecond)
	defer pollTicker.Stop()
	
	log.InfoLog.Printf("starting new session detection with interval %d ms", cfg.SessionDetectionInterval)
	
	// Perform an initial detection to get any existing sessions
	if err := detectAndAddNewSessions(instances, storage); err != nil {
		log.ErrorLog.Printf("failed to detect initial sessions: %v", err)
	}
	
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			
			// Check if the event is for a relevant file
			isStateFile := strings.Contains(event.Name, config.StateFileName)
			isInstancesFile := strings.Contains(event.Name, config.InstancesFileName)
			
			// Skip non-relevant files (but process all files in the config dir)
			if !isStateFile && !isInstancesFile && !strings.Contains(event.Name, configDir) {
				continue
			}
			
			// Handle any event that might indicate configuration has changed
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				// Skip if we processed a change very recently (debounce)
				if time.Since(lastProcessTime) < debounceTime {
					continue
				}
				lastProcessTime = time.Now()
				
				log.InfoLog.Printf("detected change in file %s, checking for new sessions", event.Name)
				if err := detectAndAddNewSessions(instances, storage); err != nil {
					log.ErrorLog.Printf("failed to detect new sessions: %v", err)
				}
				
				// Make sure we're watching the files if they were created or recreated
				if event.Has(fsnotify.Create) {
					// Re-add state file watcher if needed
					if strings.Contains(event.Name, config.StateFileName) {
						if err := watcher.Add(stateFilePath); err != nil {
							log.WarningLog.Printf("failed to add state file to watcher after creation: %v", err)
						}
					}
					
					// Re-add instances file watcher if needed
					instancesPath := filepath.Join(configDir, config.InstancesFileName)
					if strings.Contains(event.Name, config.InstancesFileName) {
						if err := watcher.Add(instancesPath); err != nil {
							log.WarningLog.Printf("failed to add instances file to watcher after creation: %v", err)
						}
					}
				}
			}
			
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.ErrorLog.Printf("watcher error: %v", err)
			
		case <-pollTicker.C:
			// Periodically poll for new sessions as a fallback mechanism
			if err := detectAndAddNewSessions(instances, storage); err != nil {
				log.ErrorLog.Printf("failed to detect new sessions during polling: %v", err)
			}
			
		case <-stopCh:
			log.InfoLog.Printf("stopping session detection")
			return
		}
	}
}

// detectAndAddNewSessions checks for new sessions and adds them to the current instances
func detectAndAddNewSessions(currentInstances *[]*session.Instance, storage *session.Storage) error {
	// State refreshing is now handled automatically in GetInstances()
	
	// Load the latest instances from the refreshed state
	newlyLoadedInstances, err := storage.LoadInstances()
	if err != nil {
		return fmt.Errorf("failed to load instances from state: %w", err)
	}
	
	// Create a map of existing instance titles for quick lookup
	existingTitles := make(map[string]bool)
	for _, instance := range *currentInstances {
		existingTitles[instance.Title] = true
	}
	
	// Find any new instances not in our current list
	newInstances := []*session.Instance{}
	for _, instance := range newlyLoadedInstances {
		if !existingTitles[instance.Title] {
			log.InfoLog.Printf("detected new session: %s (status: %d)", instance.Title, instance.Status)
			
			// Only add the instance if it's been properly started
			if instance.Started() {
				// Assume AutoYes is true if the daemon is running
				instance.AutoYes = true
				newInstances = append(newInstances, instance)
			} else {
				log.InfoLog.Printf("skipping new session %s because it's not started (status: %d)", instance.Title, instance.Status)
			}
		}
	}
	
	// Add new instances to our current list if any found
	if len(newInstances) > 0 {
		*currentInstances = append(*currentInstances, newInstances...)
		log.InfoLog.Printf("added %d new session(s) to daemon", len(newInstances))
		
		// Log all current sessions for debugging
		log.InfoLog.Printf("current sessions (%d total):", len(*currentInstances))
		for i, instance := range *currentInstances {
			log.InfoLog.Printf("  [%d] %s (status: %d)", i, instance.Title, instance.Status)
		}
	}
	
	return nil
}

// LaunchDaemon launches the daemon process.
func LaunchDaemon() error {
	// Find the claude squad binary.
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	cmd := exec.Command(execPath, "--daemon")

	// Detach the process from the parent
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	// Set process group to prevent signals from propagating
	cmd.SysProcAttr = getSysProcAttr()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start child process: %w", err)
	}

	log.InfoLog.Printf("started daemon child process with PID: %d", cmd.Process.Pid)

	// Save PID to a file for later management
	pidDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	pidFile := filepath.Join(pidDir, "daemon.pid")
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	// Don't wait for the child to exit, it's detached
	return nil
}

// StopDaemon attempts to stop a running daemon process if it exists. Returns no error if the daemon is not found
// (assumes the daemon does not exist).
func StopDaemon() error {
	pidDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	pidFile := filepath.Join(pidDir, "daemon.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return fmt.Errorf("invalid PID file format: %w", err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find daemon process: %w", err)
	}

	if err := proc.Kill(); err != nil {
		return fmt.Errorf("failed to stop daemon process: %w", err)
	}

	// Clean up PID file
	if err := os.Remove(pidFile); err != nil {
		return fmt.Errorf("failed to remove PID file: %w", err)
	}

	log.InfoLog.Printf("daemon process (PID: %d) stopped successfully", pid)
	return nil
}
