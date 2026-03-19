package services

import (
	"context"
	"fmt"
	"strings"

	"claude-squad/config"
	sessionv1 "claude-squad/gen/proto/go/session/v1"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ConfigService handles Claude configuration file RPC methods.
//
// It is dependency-light: each call creates a fresh ClaudeConfigManager so
// there is no shared state to synchronise.
type ConfigService struct{}

// NewConfigService creates a ConfigService.
func NewConfigService() *ConfigService {
	return &ConfigService{}
}

// GetClaudeConfig retrieves a Claude configuration file by name.
func (cs *ConfigService) GetClaudeConfig(
	ctx context.Context,
	req *connect.Request[sessionv1.GetClaudeConfigRequest],
) (*connect.Response[sessionv1.GetClaudeConfigResponse], error) {
	mgr, err := config.NewClaudeConfigManager()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create config manager: %w", err))
	}

	configFile, err := mgr.GetConfig(req.Msg.Filename)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&sessionv1.GetClaudeConfigResponse{
		Config: &sessionv1.ClaudeConfigFile{
			Name:    configFile.Name,
			Path:    configFile.Path,
			Content: configFile.Content,
			ModTime: timestamppb.New(configFile.ModTime),
		},
	}), nil
}

// ListClaudeConfigs returns all configuration files in the ~/.claude directory.
func (cs *ConfigService) ListClaudeConfigs(
	ctx context.Context,
	req *connect.Request[sessionv1.ListClaudeConfigsRequest],
) (*connect.Response[sessionv1.ListClaudeConfigsResponse], error) {
	mgr, err := config.NewClaudeConfigManager()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create config manager: %w", err))
	}

	configs, err := mgr.ListConfigs()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	protoConfigs := make([]*sessionv1.ClaudeConfigFile, 0, len(configs))
	for _, cfg := range configs {
		protoConfigs = append(protoConfigs, &sessionv1.ClaudeConfigFile{
			Name:    cfg.Name,
			Path:    cfg.Path,
			Content: cfg.Content,
			ModTime: timestamppb.New(cfg.ModTime),
		})
	}

	return connect.NewResponse(&sessionv1.ListClaudeConfigsResponse{
		Configs: protoConfigs,
	}), nil
}

// UpdateClaudeConfig updates a Claude configuration file with atomic write and backup.
func (cs *ConfigService) UpdateClaudeConfig(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateClaudeConfigRequest],
) (*connect.Response[sessionv1.UpdateClaudeConfigResponse], error) {
	mgr, err := config.NewClaudeConfigManager()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create config manager: %w", err))
	}

	// Use validation if requested
	if req.Msg.Validate {
		err = mgr.UpdateConfigWithValidation(req.Msg.Filename, req.Msg.Content)
	} else {
		err = mgr.UpdateConfig(req.Msg.Filename, req.Msg.Content)
	}

	if err != nil {
		if strings.Contains(err.Error(), "validation failed") {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Read back the updated file
	configFile, err := mgr.GetConfig(req.Msg.Filename)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to read updated config: %w", err))
	}

	return connect.NewResponse(&sessionv1.UpdateClaudeConfigResponse{
		Config: &sessionv1.ClaudeConfigFile{
			Name:    configFile.Name,
			Path:    configFile.Path,
			Content: configFile.Content,
			ModTime: timestamppb.New(configFile.ModTime),
		},
	}), nil
}
