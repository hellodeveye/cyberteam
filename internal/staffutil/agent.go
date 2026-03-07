package staffutil

import (
	"fmt"

	"cyberteam/internal/agent"
	"cyberteam/internal/llm"
	"cyberteam/internal/tools"
)

// AgentConfig Agent 配置 (适配器版本)
type AgentConfig struct {
	Name          string
	Model         string
	LLMClient     llm.Client
	MCPClient     *StaffMCPClient // MCP 客户端 (可选)
	SystemPrompt  string
	MaxIterations int
	Debug         bool
}

// Memory 对话记忆 (适配器版本，兼容旧接口)
type Memory struct {
	memory agent.Memory
}

// AddMessage 添加消息到记忆
func (m *Memory) AddMessage(role, content string) {
	m.memory.AddMessage(role, content)
}

// AddToolResult 添加工具结果到记忆
func (m *Memory) AddToolResult(toolName, result string) {
	m.memory.AddToolResult(toolName, result)
}

// GetMessages 获取所有消息
func (m *Memory) GetMessages() []llm.Message {
	return m.memory.GetMessages()
}

// Clear 清空记忆
func (m *Memory) Clear() {
	m.memory.Clear()
}

// Agent 智能代理 (适配器，包装 internal/agent.Agent)
type Agent struct {
	agent     *agent.Agent
	mcpClient *StaffMCPClient
	config    AgentConfig
}

// MCPAdapter 将 StaffMCPClient 适配为 agent.ToolExecutor
type MCPAdapter struct {
	Client *StaffMCPClient
}

func (a *MCPAdapter) ExecuteTool(name string, args map[string]any) (string, error) {
	if a.Client == nil {
		return "", nil
	}
	return a.Client.CallTool(name, args)
}

// BashAdapter 将 tools.BashTool 适配为 agent.ToolExecutor
type BashAdapter struct {
	Tool *tools.BashTool
}

func (a *BashAdapter) ExecuteTool(name string, args map[string]any) (string, error) {
	if a.Tool == nil {
		return "", fmt.Errorf("bash tool not initialized")
	}
	command, _ := args["command"].(string)
	if command == "" {
		return "", fmt.Errorf("command parameter required")
	}
	result := a.Tool.Execute(command)
	if !result.Success {
		return "", fmt.Errorf("%s", result.Error)
	}
	return result.Output, nil
}

// NewAgent 创建新的 Agent 实例 (使用 internal/agent)
func NewAgent(config AgentConfig) *Agent {
	// 创建 MCP 适配器
	var mcpExecutor agent.ToolExecutor
	if config.MCPClient != nil {
		mcpExecutor = &MCPAdapter{Client: config.MCPClient}
	}

	// 创建 internal/agent.Agent
	coreAgent := agent.New(agent.Config{
		Name:          config.Name,
		Model:         config.Model,
		LLMClient:     config.LLMClient,
		MCPExecutor:   mcpExecutor,
		SystemPrompt:  config.SystemPrompt,
		MaxIterations: config.MaxIterations,
		Debug:         config.Debug,
	})

	return &Agent{
		agent:     coreAgent,
		mcpClient: config.MCPClient,
		config:    config,
	}
}

// SetSystemPrompt 设置系统提示
func (a *Agent) SetSystemPrompt(prompt string) {
	a.agent.SetSystemPrompt(prompt)
}

// AddToolResult 添加工具结果到记忆
func (a *Agent) AddToolResult(toolName, result string) {
	a.agent.Memory().AddToolResult(toolName, result)
}

// Execute 执行对话
func (a *Agent) Execute(userPrompt string) string {
	return a.agent.Execute(userPrompt)
}

// ExecuteWithContext 带上下文的执行
func (a *Agent) ExecuteWithContext(transcript, currentMessage string) string {
	return a.agent.ExecuteWithContext(transcript, currentMessage)
}
