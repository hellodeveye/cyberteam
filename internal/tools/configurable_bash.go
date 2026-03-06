package tools

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"cyberteam/internal/profile"
)

// ConfigurableBashTool 基于配置的可配置 Bash 工具
type ConfigurableBashTool struct {
	*BashTool
	config *profile.BashConfig
}

// NewConfigurableBashTool 从 Profile 配置创建 Bash 工具
func NewConfigurableBashTool(workspacesDir, projectName, projectID, stage string, toolsConfig profile.ToolsConfig) (*ConfigurableBashTool, error) {
	// 检查是否启用 bash
	if toolsConfig.Bash == nil || !toolsConfig.Bash.Enabled {
		return nil, fmt.Errorf("bash tool not enabled in profile")
	}

	// 创建基础工具
	projectDir := filepath.Join(workspacesDir,
		sanitize(projectName)+"-"+projectID[:8],
		stage)

	base := NewBashTool(projectDir)

	// 应用配置
	config := toolsConfig.Bash

	// 合并允许的命令
	if len(config.Allow) > 0 {
		for _, cmd := range config.Allow {
			base.allowedCmds[cmd] = true
		}
	}

	// 合并禁止的命令
	if len(config.Deny) > 0 {
		for _, cmd := range config.Deny {
			base.blockedCmds[cmd] = true
		}
	}

	// 设置超时
	if config.Timeout != "" {
		if d, err := time.ParseDuration(config.Timeout); err == nil {
			base.timeout = d
		}
	}

	// 设置最大输出
	if config.MaxOutput > 0 {
		base.maxOutput = config.MaxOutput
	}

	return &ConfigurableBashTool{
		BashTool: base,
		config:   config,
	}, nil
}

// IsAllowed 检查命令是否被允许
func (c *ConfigurableBashTool) IsAllowed(command string) bool {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return false
	}
	cmd := parts[0]

	// 先检查禁止列表
	if c.blockedCmds[cmd] {
		return false
	}

	// 再检查允许列表（如果声明了允许列表，只允许列表内的）
	if len(c.config.Allow) > 0 {
		return c.allowedCmds[cmd]
	}

	// 没有声明允许列表，使用默认白名单
	return true
}
