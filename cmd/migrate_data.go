// +build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"claude-squad/config"
	"claude-squad/session"
)

func main() {
	// Get config directory
	configDir, err := config.GetConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get config directory: %v\n", err)
		os.Exit(1)
	}

	statePath := filepath.Join(configDir, "state.json")
	dbPath := filepath.Join(configDir, "sessions.db")

	fmt.Printf("Reading state from: %s\n", statePath)
	fmt.Printf("Target database: %s\n", dbPath)

	// Read state.json
	stateData, err := os.ReadFile(statePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read state.json: %v\n", err)
		os.Exit(1)
	}

	// Parse state.json to extract instances
	var state struct {
		HelpScreensSeen uint32                `json:"help_screens_seen"`
		Instances       []session.InstanceData `json:"instances"`
	}
	if err := json.Unmarshal(stateData, &state); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse state.json: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d sessions in state.json\n", len(state.Instances))

	// Create temporary JSON file with just instances for migration
	tmpPath := filepath.Join(configDir, "instances_temp.json")
	instancesJSON, err := json.MarshalIndent(state.Instances, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal instances: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(tmpPath, instancesJSON, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write temporary instances file: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(tmpPath)

	// Check if database already exists
	if _, err := os.Stat(dbPath); err == nil {
		fmt.Printf("\nWARNING: Database already exists at %s\n", dbPath)
		fmt.Print("Do you want to overwrite it? (yes/no): ")
		var response string
		fmt.Scanln(&response)
		if response != "yes" {
			fmt.Println("Migration cancelled")
			os.Exit(0)
		}
	}

	// Run migration
	fmt.Println("\nStarting migration...")
	result, err := session.MigrateJSONToEnt(session.MigrationOptions{
		JSONPath:       tmpPath,
		SQLitePath:     dbPath,
		ForceOverwrite: true,
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Migration failed: %v\n", err)
		os.Exit(1)
	}

	// Print results
	fmt.Println("\n=== Migration Results ===")
	fmt.Printf("Total sessions: %d\n", result.TotalSessions)
	fmt.Printf("Migrated: %d\n", result.MigratedSessions)
	fmt.Printf("Skipped: %d\n", result.SkippedSessions)
	fmt.Printf("Duration: %v\n", result.Duration)
	fmt.Printf("Backup created: %v\n", result.BackupCreated)
	if result.BackupCreated {
		fmt.Printf("Backup path: %s\n", result.BackupPath)
	}

	if len(result.Errors) > 0 {
		fmt.Printf("\nErrors encountered: %d\n", len(result.Errors))
		for _, errMsg := range result.Errors {
			fmt.Printf("  - %s\n", errMsg)
		}
	}

	// Validate migration
	fmt.Println("\nValidating migration...")
	if err := session.ValidateEntMigration(tmpPath, dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Migration validated successfully!")
	fmt.Println("\nAll session data has been migrated to Ent.")
	fmt.Println("The original state.json has been backed up.")
}
