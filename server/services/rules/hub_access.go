// Package rules holds the auto-approval rules: the rule store, the RulesService RPCs,
// classification analytics and the rule-generation prompts. It never imports server/services.
package rules

import (
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/services/claudesettings"
	"github.com/tstapler/stapler-squad/session"
)

// NewEmptyRulesStore returns a store with no rules loaded: the fallback for a caller
// whose NewRulesStore failed.
func NewEmptyRulesStore(storage *session.Storage) *RulesStore {
	return &RulesStore{storage: storage}
}

// Classifier returns the live classifier that rule changes rebuild.
func (rs *RulesService) Classifier() *classifier.RuleBasedClassifier { return rs.classifier }

// AnalyticsStore returns the store passed to NewRulesService.
func (rs *RulesService) AnalyticsStore() *AnalyticsStore { return rs.analyticsStore }

// ClaudeSettingsWatcher returns the watcher set by SetClaudeSettingsWatcher, or nil.
func (rs *RulesService) ClaudeSettingsWatcher() *claudesettings.ClaudeSettingsWatcher {
	return rs.claudeSettingsWatcher
}

// ReconcileNow runs one synchronous pass over the pending approvals; rule changes
// otherwise run it in the background.
func (rs *RulesService) ReconcileNow() { rs.reconcilePendingApprovalsSafe() }

// DisableAntigravityExport stops rule changes from being written under $HOME, so
// parallel tests never write into another test's HOME.
func (rs *RulesService) DisableAntigravityExport() { rs.exportAntigravity = nil }
