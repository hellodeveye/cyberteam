package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"cyberteam/internal/protocol"
)

// ServerInterface 内置 Server 接口
type ServerInterface interface {
	Name() string
	Start() error
	Stop() error
	IsReady() bool
	Tools() []Tool
	CallTool(name string, args map[string]interface{}) (*JSONRPCResponse, error)
}

// Manager MCP 管理器
type Manager struct {
	config          *Config
	servers         map[string]*ServerInstance
	internalServers map[string]ServerInterface // 内置 Server（不启动进程）
	mu              sync.RWMutex
	logger          func(string, ...interface{})
}

// NewManager 创建 MCP 管理器
func NewManager(configPath string) (*Manager, error) {
	// 如果配置文件不存在，使用默认配置
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &Manager{
			config:          &Config{Settings: Settings{Timeout: 30}},
			servers:        make(map[string]*ServerInstance),
			internalServers: make(map[string]ServerInterface),
			logger:         defaultLogger,
		}, nil
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	return &Manager{
		config:          config,
		servers:        make(map[string]*ServerInstance),
		internalServers: make(map[string]ServerInterface),
		logger:         defaultLogger,
	}, nil
}

// StartAll 启动所有启用的 MCP Server
func (m *Manager) StartAll() error {
	enabled := m.config.GetEnabledServers()
	if len(enabled) == 0 {
		m.logger("[MCP] 没有启用的 Server")
		return nil
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(enabled))

	for name, serverConfig := range enabled {
		wg.Add(1)
		go func(name string, config Server) {
			defer wg.Done()

			instance := NewServerInstance(name, config)
			if err := instance.Start(); err != nil {
				errChan <- fmt.Errorf("start %s: %w", name, err)
				return
			}

			m.mu.Lock()
			m.servers[name] = instance
			m.mu.Unlock()
		}(name, serverConfig)
	}

	wg.Wait()
	close(errChan)

	var errs []string
	for err := range errChan {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf("some servers failed: %s", strings.Join(errs, "; "))
	}

	return nil
}

// StopAll 停止所有 MCP Server
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, server := range m.servers {
		if err := server.Stop(); err != nil {
			m.logger("[MCP:%s] 停止失败: %v", name, err)
		}
	}
	m.servers = make(map[string]*ServerInstance)
}

// CallTool 调用工具（带权限检查）
func (m *Manager) CallTool(serverName, toolName, role string, args map[string]interface{}) (*protocol.MCPCallResult, error) {
	// 查找 Server
	m.mu.RLock()
	server, ok := m.servers[serverName]
	serverConfig, configOk := m.config.Servers[serverName]
	m.mu.RUnlock()

	if !ok || !configOk {
		return nil, fmt.Errorf("server not found: %s", serverName)
	}

	// 权限检查
	if !serverConfig.IsToolAllowed(toolName, role) {
		return nil, fmt.Errorf("permission denied: %s cannot use %s:%s", role, serverName, toolName)
	}

	// 调用工具
	resp, err := server.CallTool(toolName, args)
	if err != nil {
		return &protocol.MCPCallResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	if resp.Error != nil {
		return &protocol.MCPCallResult{
			Success: false,
			Error:   resp.Error.Message,
		}, nil
	}

	// 解析结果
	result := m.parseToolResult(resp.Result)
	return &protocol.MCPCallResult{
		Success: true,
		Result:  result,
	}, nil
}

// CallToolByName 通过工具名调用（自动查找 Server）
func (m *Manager) CallToolByName(fullToolName, role string, args map[string]interface{}) (*protocol.MCPCallResult, error) {
	// 解析工具名，格式: "server:tool" 或 "tool"
	parts := strings.SplitN(fullToolName, ":", 2)

	if len(parts) == 2 {
		// 明确指定了 server:tool
		return m.CallTool(parts[0], parts[1], role, args)
	}

	// 只给了 tool 名，需要查找哪个 server 有这个工具
	toolName := parts[0]

	m.mu.RLock()
	servers := make(map[string]*ServerInstance)
	for k, v := range m.servers {
		servers[k] = v
	}
	m.mu.RUnlock()

	for serverName, server := range servers {
		for _, tool := range server.Tools {
			if tool.Name == toolName {
				return m.CallTool(serverName, toolName, role, args)
			}
		}
	}

	return nil, fmt.Errorf("tool not found: %s", toolName)
}

// ListTools 列出角色可用的工具
func (m *Manager) ListTools(role string) []protocol.MCPToolInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var tools []protocol.MCPToolInfo

	for serverName, server := range m.servers {
		serverConfig, ok := m.config.Servers[serverName]
		if !ok || !serverConfig.Enabled {
			continue
		}

		// 检查角色权限
		roleAllowed := false
		for _, r := range serverConfig.ACL.Roles {
			if r == role {
				roleAllowed = true
				break
			}
		}
		if !roleAllowed {
			continue
		}

		// 添加该 Server 的工具
		for _, tool := range server.Tools {
			// 检查工具是否允许
			if !serverConfig.IsToolAllowed(tool.Name, role) {
				continue
			}

			tools = append(tools, protocol.MCPToolInfo{
				Name:        fmt.Sprintf("%s:%s", serverName, tool.Name),
				Server:      serverName,
				Description: tool.Description,
				InputSchema: tool.InputSchema,
			})
		}
	}

	return tools
}

// ListToolsString 列出工具（字符串格式，用于 Prompt）
func (m *Manager) ListToolsString(role string) string {
	tools := m.ListTools(role)
	if len(tools) == 0 {
		return "（暂无可用的外部工具）"
	}

	var lines []string
	for _, t := range tools {
		lines = append(lines, fmt.Sprintf("- %s: %s", t.Name, t.Description))
	}
	return strings.Join(lines, "\n")
}

// GetServerStatus 获取 Server 状态
func (m *Manager) GetServerStatus() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]string)
	for name, server := range m.servers {
		if server.IsReady() {
			status[name] = "ready"
		} else {
			status[name] = "not ready"
		}
	}
	return status
}

// parseToolResult 解析工具返回结果
func (m *Manager) parseToolResult(result map[string]interface{}) string {
	if content, ok := result["content"].([]interface{}); ok {
		var texts []string
		for _, item := range content {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if text, ok := itemMap["text"].(string); ok {
					texts = append(texts, text)
				}
			}
		}
		return strings.Join(texts, "\n")
	}

	// 尝试直接返回 result 的 JSON
	data, _ := json.Marshal(result)
	return string(data)
}

// defaultLogger 默认日志函数
func defaultLogger(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// RegisterInternalServer 注册内置 Server
func (m *Manager) RegisterInternalServer(name string, server ServerInterface) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.internalServers[name] = server
	m.logger("[MCP] registered internal server: %s", name)
}

// GetInternalServer 获取内置 Server
func (m *Manager) GetInternalServer(name string) (ServerInterface, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	server, ok := m.internalServers[name]
	return server, ok
}

// ListInternalTools 列出内置 Server 的工具
func (m *Manager) ListInternalTools() []protocol.MCPToolInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var tools []protocol.MCPToolInfo
	for serverName, server := range m.internalServers {
		if !server.IsReady() {
			continue
		}
		for _, tool := range server.Tools() {
			tools = append(tools, protocol.MCPToolInfo{
				Name:        fmt.Sprintf("%s:%s", serverName, tool.Name),
				Server:      serverName,
				Description: tool.Description,
				InputSchema: tool.InputSchema,
			})
		}
	}
	return tools
}

// CallInternalTool 调用内置工具
func (m *Manager) CallInternalTool(serverName, toolName string, args map[string]interface{}) (*protocol.MCPCallResult, error) {
	m.mu.RLock()
	server, ok := m.internalServers[serverName]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("internal server not found: %s", serverName)
	}

	resp, err := server.CallTool(toolName, args)
	if err != nil {
		return &protocol.MCPCallResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	if resp.Error != nil {
		return &protocol.MCPCallResult{
			Success: false,
			Error:   resp.Error.Message,
		}, nil
	}

	// 提取内置工具的 success/error 状态
	success := true
	errMsg := ""
	if s, ok := resp.Result["success"].(bool); ok {
		success = s
	}
	if e, ok := resp.Result["error"].(string); ok && e != "" {
		errMsg = e
	}

	result := m.parseToolResult(resp.Result)
	return &protocol.MCPCallResult{
		Success: success,
		Result:  result,
		Error:   errMsg,
	}, nil
}
