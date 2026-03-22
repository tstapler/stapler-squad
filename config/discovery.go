package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/tstapler/stapler-squad/log"
)

// DiscoveryMode defines how the application discovers Claude instances
type DiscoveryMode string

const (
	// DiscoveryManagedOnly discovers only instances created by stapler-squad
	DiscoveryManagedOnly DiscoveryMode = "managed-only"

	// DiscoveryExternalOnly discovers only external Claude instances
	DiscoveryExternalOnly DiscoveryMode = "external-only"

	// DiscoveryAll discovers both managed and external instances
	DiscoveryAll DiscoveryMode = "all"
)

const DiscoveryConfigFileName = "discovery.json"

// DiscoveryConfig controls instance discovery behavior and safety settings
type DiscoveryConfig struct {
	// Mode determines which types of instances to discover
	Mode DiscoveryMode `json:"mode"`

	// AllowExternalAttach controls whether users can attach to external instances
	AllowExternalAttach bool `json:"allow_external_attach"`

	// ConfirmExternalOperations requires confirmation before operations on external instances
	ConfirmExternalOperations bool `json:"confirm_external_operations"`

	// SocketPaths defines custom socket paths for discovery (optional)
	// Empty means use system defaults (/tmp/tmux-*/default)
	SocketPaths []string `json:"socket_paths"`

	// ExcludedSocketPaths defines socket paths to skip during discovery
	ExcludedSocketPaths []string `json:"excluded_socket_paths"`

	// DiscoverInterval is the interval (ms) at which external instances are scanned
	DiscoverInterval int `json:"discover_interval"`

	// AutoRefreshExternal enables automatic refresh of external instance metadata
	AutoRefreshExternal bool `json:"auto_refresh_external"`
}

// DefaultDiscoveryConfig returns the default discovery configuration
// By default, only managed instances are shown for safety
func DefaultDiscoveryConfig() *DiscoveryConfig {
	return &DiscoveryConfig{
		Mode:                      DiscoveryManagedOnly, // Safe default - only show managed instances
		AllowExternalAttach:       false,                // Don't allow attaching to external by default
		ConfirmExternalOperations: true,                 // Always confirm external operations
		SocketPaths:               []string{},           // Use system defaults
		ExcludedSocketPaths:       []string{},           // No exclusions by default
		DiscoverInterval:          10000,                // 10 seconds
		AutoRefreshExternal:       true,                 // Keep external metadata fresh
	}
}

// LoadDiscoveryConfig loads the discovery configuration from disk
func LoadDiscoveryConfig() *DiscoveryConfig {
	configDir, err := GetConfigDir()
	if err != nil {
		log.ErrorLog.Printf("failed to get config directory: %v", err)
		return DefaultDiscoveryConfig()
	}

	configPath := filepath.Join(configDir, DiscoveryConfigFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create and save default config if file doesn't exist
			defaultCfg := DefaultDiscoveryConfig()
			if saveErr := SaveDiscoveryConfig(defaultCfg); saveErr != nil {
				log.WarningLog.Printf("failed to save default discovery config: %v", saveErr)
			}
			return defaultCfg
		}

		log.WarningLog.Printf("failed to read discovery config file: %v", err)
		return DefaultDiscoveryConfig()
	}

	var config DiscoveryConfig
	if err := json.Unmarshal(data, &config); err != nil {
		log.ErrorLog.Printf("failed to parse discovery config file: %v", err)
		return DefaultDiscoveryConfig()
	}

	// Validate mode and use default if invalid
	if config.Mode != DiscoveryManagedOnly &&
	   config.Mode != DiscoveryExternalOnly &&
	   config.Mode != DiscoveryAll {
		log.WarningLog.Printf("invalid discovery mode '%s', using default", config.Mode)
		config.Mode = DiscoveryManagedOnly
	}

	return &config
}

// SaveDiscoveryConfig saves the discovery configuration to disk
func SaveDiscoveryConfig(config *DiscoveryConfig) error {
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(configDir, DiscoveryConfigFileName)
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// IsExternalDiscoveryEnabled returns true if external instance discovery is enabled
func (c *DiscoveryConfig) IsExternalDiscoveryEnabled() bool {
	return c.Mode == DiscoveryExternalOnly || c.Mode == DiscoveryAll
}

// IsManagedDiscoveryEnabled returns true if managed instance discovery is enabled
func (c *DiscoveryConfig) IsManagedDiscoveryEnabled() bool {
	return c.Mode == DiscoveryManagedOnly || c.Mode == DiscoveryAll
}

// ShouldShowExternalInstances returns true if external instances should be displayed
func (c *DiscoveryConfig) ShouldShowExternalInstances() bool {
	return c.IsExternalDiscoveryEnabled()
}

// ShouldConfirmOperation returns true if the operation requires user confirmation
func (c *DiscoveryConfig) ShouldConfirmOperation(isExternal bool) bool {
	// Always confirm operations on external instances if configured
	return isExternal && c.ConfirmExternalOperations
}

// CanAttachToExternal returns true if attaching to external instances is allowed
func (c *DiscoveryConfig) CanAttachToExternal() bool {
	return c.AllowExternalAttach && c.IsExternalDiscoveryEnabled()
}
