package mcp

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

// Config MCP 配置文件
type Config struct {
	Version     string            `yaml:"version"`
	Description string            `yaml:"description"`
	Settings    Settings          `yaml:"settings"`
	Servers     map[string]Server `yaml:"servers"`
}

type Settings struct {
	Timeout       time.Duration `yaml:"timeout"`
	MaxConcurrent int           `yaml:"max_concurrent"`
	Logging       bool          `yaml:"logging"`
}

// Server 单个 MCP Server 配置
type Server struct {
	Enabled     bool              `yaml:"enabled"`
	Description string            `yaml:"description"`
	Command     string            `yaml:"command"`
	Env         map[string]string `yaml:"env"`
	ACL         ACL               `yaml:"acl"`
}

// ACL 访问控制
type ACL struct {
	Roles        []string `yaml:"roles"`
	AllowedTools []string `yaml:"allowed_tools"`
	DeniedTools  []string `yaml:"denied_tools"`
}

// LoadConfig 加载 MCP 配置文件
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// 设置默认值
	if config.Settings.Timeout == 0 {
		config.Settings.Timeout = 30 * time.Second
	}
	if config.Settings.MaxConcurrent == 0 {
		config.Settings.MaxConcurrent = 10
	}

	// 处理环境变量
	for name, server := range config.Servers {
		for key, value := range server.Env {
			expanded := expandEnv(value)
			server.Env[key] = expanded
		}
		config.Servers[name] = server
	}

	return &config, nil
}

// expandEnv 扩展环境变量 ${VAR} 或 $VAR
func expandEnv(s string) string {
	re := regexp.MustCompile(`\$\{([^}]+)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)
	return re.ReplaceAllStringFunc(s, func(match string) string {
		// 去掉 ${} 或 $
		var varName string
		if len(match) > 2 && match[0] == '$' && match[1] == '{' {
			varName = match[2 : len(match)-1]
		} else {
			varName = match[1:]
		}
		return os.Getenv(varName)
	})
}

// IsToolAllowed 检查工具是否允许使用
func (s *Server) IsToolAllowed(toolName, role string) bool {
	// 检查角色
	roleAllowed := false
	for _, r := range s.ACL.Roles {
		if r == role {
			roleAllowed = true
			break
		}
	}
	if !roleAllowed {
		return false
	}

	// 检查禁止列表
	for _, denied := range s.ACL.DeniedTools {
		if denied == toolName {
			return false
		}
	}

	// 检查允许列表（如果配置了）
	if len(s.ACL.AllowedTools) > 0 {
		for _, allowed := range s.ACL.AllowedTools {
			if allowed == toolName {
				return true
			}
		}
		return false
	}

	return true
}

// GetEnabledServers 获取启用的 Server 列表
func (c *Config) GetEnabledServers() map[string]Server {
	result := make(map[string]Server)
	for name, server := range c.Servers {
		if server.Enabled {
			result[name] = server
		}
	}
	return result
}

// ListToolsForRole 列出角色可用的工具
func (c *Config) ListToolsForRole(role string) []ToolInfo {
	var tools []ToolInfo
	for serverName, server := range c.Servers {
		if !server.Enabled {
			continue
		}

		// 检查角色权限
		roleAllowed := false
		for _, r := range server.ACL.Roles {
			if r == role {
				roleAllowed = true
				break
			}
		}
		if !roleAllowed {
			continue
		}

		// 添加该 Server 的工具
		tools = append(tools, ToolInfo{
			Server:      serverName,
			Description: server.Description,
			Allowed:     server.ACL.AllowedTools,
		})
	}
	return tools
}

// ToolInfo 工具信息
type ToolInfo struct {
	Server      string
	Description string
	Allowed     []string
}
