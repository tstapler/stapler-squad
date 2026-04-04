package config

import (
	"testing"
)

func TestKeyCategories(t *testing.T) {
	config := DefaultConfig()

	// Test default key categories are loaded
	if config.KeyCategories == nil {
		t.Error("Expected default key categories to be loaded")
	}

	// Test some default mappings
	expectedMappings := map[string]string{
		"n":   "Session Management",
		"g":   "Git Integration",
		"up":  "Navigation",
		"f":   "Organization",
		"tab": "System",
	}

	for key, expectedCategory := range expectedMappings {
		if category := config.GetKeyCategoryForKey(key); category != expectedCategory {
			t.Errorf("Expected key %s to have category %s, got %s", key, expectedCategory, category)
		}
	}
}

func TestKeyManipulation(t *testing.T) {
	config := &Config{}

	// Test setting a key category
	config.SetKeyCategory("x", "Custom Category")
	if category := config.GetKeyCategoryForKey("x"); category != "Custom Category" {
		t.Errorf("Expected category 'Custom Category', got %s", category)
	}

	// Test getting non-existent key
	if category := config.GetKeyCategoryForKey("nonexistent"); category != "" {
		t.Errorf("Expected empty string for non-existent key, got %s", category)
	}

	// Test removing key category
	config.RemoveKeyCategory("x")
	if category := config.GetKeyCategoryForKey("x"); category != "" {
		t.Errorf("Expected empty string after removal, got %s", category)
	}
}

func TestConfigWithNilKeyCategories(t *testing.T) {
	config := &Config{
		KeyCategories: nil,
	}

	// Test getting from nil map
	if category := config.GetKeyCategoryForKey("test"); category != "" {
		t.Errorf("Expected empty string from nil map, got %s", category)
	}

	// Test setting in nil map (should initialize)
	config.SetKeyCategory("test", "Test Category")
	if config.KeyCategories == nil {
		t.Error("Expected KeyCategories map to be initialized after setting")
	}

	if category := config.GetKeyCategoryForKey("test"); category != "Test Category" {
		t.Errorf("Expected 'Test Category', got %s", category)
	}
}
