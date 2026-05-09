package services

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DefaultsService handles session defaults RPC methods.
type DefaultsService struct{}

// NewDefaultsService creates a DefaultsService.
func NewDefaultsService() *DefaultsService {
	return &DefaultsService{}
}

// GetSessionDefaults returns the full session defaults configuration.
func (d *DefaultsService) GetSessionDefaults(
	ctx context.Context,
	req *connect.Request[sessionv1.GetSessionDefaultsRequest],
) (*connect.Response[sessionv1.GetSessionDefaultsResponse], error) {
	cfg := config.LoadConfig()
	return connect.NewResponse(&sessionv1.GetSessionDefaultsResponse{
		Defaults: sessionDefaultsToProto(cfg),
	}), nil
}

// ResolveDefaults merges all default layers for the given working directory and profile.
func (d *DefaultsService) ResolveDefaults(
	ctx context.Context,
	req *connect.Request[sessionv1.ResolveDefaultsRequest],
) (*connect.Response[sessionv1.ResolveDefaultsResponse], error) {
	cfg := config.LoadConfig()
	resolved := config.ResolveDefaults(cfg, req.Msg.WorkingDir, req.Msg.ProfileName)

	resp := &sessionv1.ResolveDefaultsResponse{
		Program:          resolved.Program,
		AutoYes:          resolved.AutoYes,
		Tags:             resolved.Tags,
		EnvVars:          resolved.EnvVars,
		CliFlags:         resolved.CLIFlags,
		UsedGlobal:       resolved.UsedGlobal,
		UsedDirectory:    resolved.UsedDirectory,
		UsedProfile:      resolved.UsedProfile,
		MatchedDirectory: resolved.MatchedDirectory,
	}
	if resp.EnvVars == nil {
		resp.EnvVars = make(map[string]string)
	}
	return connect.NewResponse(resp), nil
}

// UpdateGlobalDefaults replaces the global default fields and persists them.
func (d *DefaultsService) UpdateGlobalDefaults(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateGlobalDefaultsRequest],
) (*connect.Response[sessionv1.UpdateGlobalDefaultsResponse], error) {
	cfg := config.LoadConfig()

	cfg.SessionDefaults.Program = req.Msg.Program
	cfg.SessionDefaults.AutoYes = req.Msg.AutoYes
	cfg.SessionDefaults.Tags = req.Msg.Tags
	cfg.SessionDefaults.CLIFlags = req.Msg.CliFlags
	cfg.OneOffBaseDir = req.Msg.OneOffBaseDir
	cfg.NewProjectBaseDir = req.Msg.NewProjectBaseDir
	if req.Msg.EnvVars != nil {
		cfg.SessionDefaults.EnvVars = req.Msg.EnvVars
	} else {
		cfg.SessionDefaults.EnvVars = make(map[string]string)
	}

	if err := config.SaveConfig(cfg); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save config: %w", err))
	}

	log.InfoLog.Printf("Updated global session defaults: program=%q tags=%v", cfg.SessionDefaults.Program, cfg.SessionDefaults.Tags)
	return connect.NewResponse(&sessionv1.UpdateGlobalDefaultsResponse{
		Defaults: sessionDefaultsToProto(cfg),
	}), nil
}

// UpsertProfile creates or updates a named profile.
func (d *DefaultsService) UpsertProfile(
	ctx context.Context,
	req *connect.Request[sessionv1.UpsertProfileRequest],
) (*connect.Response[sessionv1.UpsertProfileResponse], error) {
	if req.Msg.Profile == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("profile is required"))
	}
	if req.Msg.Profile.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("profile name is required"))
	}

	cfg := config.LoadConfig()

	now := time.Now()
	p := config.ProfileDefaults{
		Name:        req.Msg.Profile.Name,
		Description: req.Msg.Profile.Description,
		Program:     req.Msg.Profile.Program,
		AutoYes:     req.Msg.Profile.AutoYes,
		Tags:        req.Msg.Profile.Tags,
		EnvVars:     req.Msg.Profile.EnvVars,
		CLIFlags:    req.Msg.Profile.CliFlags,
		UpdatedAt:   now,
	}
	if req.Msg.Profile.EnvVars == nil {
		p.EnvVars = make(map[string]string)
	}
	if req.Msg.Profile.Tags == nil {
		p.Tags = []string{}
	}

	// Preserve CreatedAt if updating an existing profile.
	if existing, ok := cfg.SessionDefaults.Profiles[p.Name]; ok {
		p.CreatedAt = existing.CreatedAt
	} else {
		p.CreatedAt = now
	}

	cfg.SessionDefaults.Profiles[p.Name] = p

	if err := config.SaveConfig(cfg); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save config: %w", err))
	}

	log.InfoLog.Printf("Upserted session profile: %q", p.Name)
	return connect.NewResponse(&sessionv1.UpsertProfileResponse{
		Profile: profileDefaultsToProto(p),
	}), nil
}

// DeleteProfile removes a named profile by name.
func (d *DefaultsService) DeleteProfile(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteProfileRequest],
) (*connect.Response[sessionv1.DeleteProfileResponse], error) {
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("profile name is required"))
	}

	cfg := config.LoadConfig()

	if _, ok := cfg.SessionDefaults.Profiles[req.Msg.Name]; !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("profile %q not found", req.Msg.Name))
	}
	delete(cfg.SessionDefaults.Profiles, req.Msg.Name)

	if err := config.SaveConfig(cfg); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save config: %w", err))
	}

	log.InfoLog.Printf("Deleted session profile: %q", req.Msg.Name)
	return connect.NewResponse(&sessionv1.DeleteProfileResponse{}), nil
}

// UpsertDirectoryRule creates or updates a directory rule (matched by path).
func (d *DefaultsService) UpsertDirectoryRule(
	ctx context.Context,
	req *connect.Request[sessionv1.UpsertDirectoryRuleRequest],
) (*connect.Response[sessionv1.UpsertDirectoryRuleResponse], error) {
	if req.Msg.Rule == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("rule is required"))
	}
	if req.Msg.Rule.Path == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("rule path is required"))
	}

	cfg := config.LoadConfig()

	rule := config.DirectoryRule{
		Path:    req.Msg.Rule.Path,
		Profile: req.Msg.Rule.Profile,
	}
	if req.Msg.Rule.Overrides != nil {
		rule.Overrides = protoToProfileDefaults(req.Msg.Rule.Overrides)
	}

	// Replace existing rule with same path or append.
	found := false
	for i, r := range cfg.SessionDefaults.DirectoryRules {
		if r.Path == rule.Path {
			cfg.SessionDefaults.DirectoryRules[i] = rule
			found = true
			break
		}
	}
	if !found {
		cfg.SessionDefaults.DirectoryRules = append(cfg.SessionDefaults.DirectoryRules, rule)
	}

	if err := config.SaveConfig(cfg); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save config: %w", err))
	}

	log.InfoLog.Printf("Upserted directory rule: %q", rule.Path)
	return connect.NewResponse(&sessionv1.UpsertDirectoryRuleResponse{
		Rule: directoryRuleToProto(rule),
	}), nil
}

// DeleteDirectoryRule removes a directory rule by path.
func (d *DefaultsService) DeleteDirectoryRule(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteDirectoryRuleRequest],
) (*connect.Response[sessionv1.DeleteDirectoryRuleResponse], error) {
	if req.Msg.Path == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("rule path is required"))
	}

	cfg := config.LoadConfig()

	rules := cfg.SessionDefaults.DirectoryRules
	newRules := make([]config.DirectoryRule, 0, len(rules))
	deleted := false
	for _, r := range rules {
		if r.Path == req.Msg.Path {
			deleted = true
			continue
		}
		newRules = append(newRules, r)
	}
	if !deleted {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("rule for path %q not found", req.Msg.Path))
	}
	cfg.SessionDefaults.DirectoryRules = newRules

	if err := config.SaveConfig(cfg); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save config: %w", err))
	}

	log.InfoLog.Printf("Deleted directory rule: %q", req.Msg.Path)
	return connect.NewResponse(&sessionv1.DeleteDirectoryRuleResponse{}), nil
}

// ─── Conversion helpers ──────────────────────────────────────────────────────

func sessionDefaultsToProto(cfg *config.Config) *sessionv1.SessionDefaultsConfig {
	sd := cfg.SessionDefaults
	proto := &sessionv1.SessionDefaultsConfig{
		Program:        sd.Program,
		AutoYes:        sd.AutoYes,
		Tags:           sd.Tags,
		EnvVars:        sd.EnvVars,
		CliFlags:       sd.CLIFlags,
		Profiles:       make(map[string]*sessionv1.ProfileDefaultsProto),
		DirectoryRules:   make([]*sessionv1.DirectoryRuleProto, 0, len(sd.DirectoryRules)),
		OneOffBaseDir: cfg.OneOffBaseDir,
	}
	// Use resolved defaults so the frontend receives ~/Projects rather than "" when unset.
	if resolvedNewProjectDir, err := cfg.NewProjectBaseDirOrDefault(); err == nil {
		proto.NewProjectBaseDir = resolvedNewProjectDir
	} else {
		proto.NewProjectBaseDir = cfg.NewProjectBaseDir
	}
	// Use resolved defaults so the frontend receives ~/Projects rather than "" when unset.
	if resolvedNewProjectDir, err := cfg.NewProjectBaseDirOrDefault(); err == nil {
		proto.NewProjectBaseDir = resolvedNewProjectDir
	} else {
		proto.NewProjectBaseDir = cfg.NewProjectBaseDir
	}
	if proto.EnvVars == nil {
		proto.EnvVars = make(map[string]string)
	}
	for name, p := range sd.Profiles {
		proto.Profiles[name] = profileDefaultsToProto(p)
	}
	for _, r := range sd.DirectoryRules {
		proto.DirectoryRules = append(proto.DirectoryRules, directoryRuleToProto(r))
	}
	return proto
}

func profileDefaultsToProto(p config.ProfileDefaults) *sessionv1.ProfileDefaultsProto {
	proto := &sessionv1.ProfileDefaultsProto{
		Name:        p.Name,
		Description: p.Description,
		Program:     p.Program,
		AutoYes:     p.AutoYes,
		Tags:        p.Tags,
		EnvVars:     p.EnvVars,
		CliFlags:    p.CLIFlags,
		CreatedAt:   timestamppb.New(p.CreatedAt),
		UpdatedAt:   timestamppb.New(p.UpdatedAt),
	}
	if proto.EnvVars == nil {
		proto.EnvVars = make(map[string]string)
	}
	if proto.Tags == nil {
		proto.Tags = []string{}
	}
	return proto
}

func protoToProfileDefaults(p *sessionv1.ProfileDefaultsProto) config.ProfileDefaults {
	if p == nil {
		return config.ProfileDefaults{}
	}
	pd := config.ProfileDefaults{
		Name:        p.Name,
		Description: p.Description,
		Program:     p.Program,
		AutoYes:     p.AutoYes,
		Tags:        p.Tags,
		EnvVars:     p.EnvVars,
		CLIFlags:    p.CliFlags,
	}
	if p.CreatedAt != nil {
		pd.CreatedAt = p.CreatedAt.AsTime()
	}
	if p.UpdatedAt != nil {
		pd.UpdatedAt = p.UpdatedAt.AsTime()
	}
	if pd.EnvVars == nil {
		pd.EnvVars = make(map[string]string)
	}
	if pd.Tags == nil {
		pd.Tags = []string{}
	}
	return pd
}

func directoryRuleToProto(r config.DirectoryRule) *sessionv1.DirectoryRuleProto {
	proto := &sessionv1.DirectoryRuleProto{
		Path:      r.Path,
		Profile:   r.Profile,
		Overrides: profileDefaultsToProto(r.Overrides),
	}
	return proto
}
