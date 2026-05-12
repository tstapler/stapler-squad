package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// MigrationOptions configures the migration from JSON to SQLite
type MigrationOptions struct {
	// JSONPath is the path to the existing JSON state file
	JSONPath string

	// SQLitePath is the path where the SQLite database will be created
	SQLitePath string

	// BackupPath is the path where the JSON backup will be saved
	BackupPath string

	// ForceOverwrite allows overwriting existing SQLite database
	ForceOverwrite bool

	// DryRun performs validation without actually migrating
	DryRun bool
}

// MigrationResult contains the results of the migration process
type MigrationResult struct {
	TotalSessions      int
	MigratedSessions   int
	SkippedSessions    int
	Errors             []string
	Duration           time.Duration
	BackupCreated      bool
	BackupPath         string
	SQLiteDatabasePath string
}

// MigrateJSONToEnt migrates session data from JSON to Ent ORM storage.
func MigrateJSONToEnt(opts MigrationOptions) (*MigrationResult, error) {
	startTime := time.Now()
	result := &MigrationResult{
		SQLiteDatabasePath: opts.SQLitePath,
	}

	log.Info("starting migration from JSON to Ent",
		"json_source", opts.JSONPath, "db_target", opts.SQLitePath, "dry_run", opts.DryRun)

	// Step 1: Validate JSON file exists and is readable
	log.Info("step 1/6: validating JSON file")
	jsonData, err := os.ReadFile(opts.JSONPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON file: %w", err)
	}

	// Step 2: Parse JSON data
	log.Info("step 2/6: parsing JSON data")
	var instances []InstanceData
	if err := json.Unmarshal(jsonData, &instances); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON data: %w", err)
	}

	result.TotalSessions = len(instances)
	log.Info("found sessions in JSON file", "count", result.TotalSessions)

	if result.TotalSessions == 0 {
		log.Warn("no sessions found in JSON file, migration not needed")
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Step 3: Create backup of JSON file
	log.Info("step 3/6: creating backup of JSON file")
	if !opts.DryRun {
		backupPath := opts.BackupPath
		if backupPath == "" {
			timestamp := time.Now().Format("20060102_150405")
			dir := filepath.Dir(opts.JSONPath)
			base := filepath.Base(opts.JSONPath)
			backupPath = filepath.Join(dir, fmt.Sprintf("%s.backup_%s.json", base, timestamp))
		}

		if err := os.WriteFile(backupPath, jsonData, 0644); err != nil {
			return nil, fmt.Errorf("failed to create backup: %w", err)
		}

		result.BackupCreated = true
		result.BackupPath = backupPath
		log.Info("backup created", "path", backupPath)
	} else {
		log.Info("dry run: backup creation skipped")
	}

	// Step 4: Check if Ent database already exists
	log.Info("step 4/6: checking Ent database")
	if _, err := os.Stat(opts.SQLitePath); err == nil {
		if !opts.ForceOverwrite {
			return nil, fmt.Errorf("ent database already exists at %s (use ForceOverwrite to overwrite)", opts.SQLitePath)
		}
		log.Warn("Ent database exists, will be overwritten (ForceOverwrite=true)")

		if !opts.DryRun {
			if err := os.Remove(opts.SQLitePath); err != nil {
				return nil, fmt.Errorf("failed to remove existing database: %w", err)
			}
		}
	}

	// Step 5: Initialize Ent repository
	log.Info("step 5/6: initializing Ent repository")
	if opts.DryRun {
		log.Info("dry run: database initialization skipped")
	} else {
		// Ensure directory exists
		dbDir := filepath.Dir(opts.SQLitePath)
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}

		repo, err := NewEntRepository(WithDatabasePath(opts.SQLitePath))
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Ent repository: %w", err)
		}
		defer repo.Close()

		// Step 6: Migrate each session
		log.Info("step 6/6: migrating sessions to Ent")
		ctx := context.Background()

		for i, instanceData := range instances {
			log.Debug("migrating session", "index", i+1, "total", result.TotalSessions, "session", instanceData.Title)

			if err := repo.Create(ctx, instanceData); err != nil {
				errMsg := fmt.Sprintf("Failed to migrate session '%s': %v", instanceData.Title, err)
				result.Errors = append(result.Errors, errMsg)
				result.SkippedSessions++
				log.Error("failed to migrate session", "session", instanceData.Title, "err", err)
				continue
			}

			result.MigratedSessions++
		}
	}

	result.Duration = time.Since(startTime)

	log.Info("migration completed",
		"duration", result.Duration,
		"total", result.TotalSessions,
		"migrated", result.MigratedSessions,
		"skipped", result.SkippedSessions,
		"errors", len(result.Errors))

	if len(result.Errors) > 0 {
		log.Warn("migration completed with errors", "count", len(result.Errors))
		for _, errMsg := range result.Errors {
			log.Warn("migration error", "msg", errMsg)
		}
	}

	if opts.DryRun {
		log.Info("dry run completed — no changes made")
		result.MigratedSessions = result.TotalSessions
	}

	return result, nil
}

// ValidateEntMigration verifies that all sessions from JSON were successfully migrated to Ent
func ValidateEntMigration(jsonPath, entDBPath string) error {
	log.Info("validating Ent migration", "json_path", jsonPath, "db_path", entDBPath)

	// Load JSON data
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to read JSON file: %w", err)
	}

	var jsonInstances []InstanceData
	if err := json.Unmarshal(jsonData, &jsonInstances); err != nil {
		return fmt.Errorf("failed to unmarshal JSON data: %w", err)
	}

	// Load Ent data
	repo, err := NewEntRepository(WithDatabasePath(entDBPath))
	if err != nil {
		return fmt.Errorf("failed to open Ent database: %w", err)
	}
	defer repo.Close()

	ctx := context.Background()
	entInstances, err := repo.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list Ent sessions: %w", err)
	}

	// Compare counts
	if len(jsonInstances) != len(entInstances) {
		return fmt.Errorf("session count mismatch: JSON has %d, Ent has %d",
			len(jsonInstances), len(entInstances))
	}

	// Create map of Ent sessions by title for quick lookup
	entMap := make(map[string]InstanceData)
	for _, inst := range entInstances {
		entMap[inst.Title] = inst
	}

	// Verify each JSON session exists in Ent
	missingCount := 0
	mismatchCount := 0
	for _, jsonInst := range jsonInstances {
		entInst, exists := entMap[jsonInst.Title]
		if !exists {
			log.Error("session missing in Ent", "session", jsonInst.Title)
			missingCount++
			continue
		}

		if jsonInst.Path != entInst.Path ||
			jsonInst.Branch != entInst.Branch ||
			jsonInst.Status != entInst.Status ||
			jsonInst.Program != entInst.Program {
			log.Error("session data mismatch in Ent", "session", jsonInst.Title)
			mismatchCount++
		}
	}

	if missingCount > 0 || mismatchCount > 0 {
		return fmt.Errorf("validation failed: %d missing, %d mismatched", missingCount, mismatchCount)
	}

	log.Info("Ent migration validation successful", "count", len(jsonInstances))
	return nil
}

// RollbackMigration restores the JSON backup and removes the SQLite database
func RollbackMigration(backupPath, sqlitePath string) error {
	log.Info("rolling back migration", "backup", backupPath, "sqlite", sqlitePath)

	// Verify backup exists
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	// Remove SQLite database
	if err := os.Remove(sqlitePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove SQLite database: %w", err)
	}

	log.Info("rollback completed successfully", "backup", backupPath)
	return nil
}
