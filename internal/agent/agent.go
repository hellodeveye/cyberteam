package agent

import (
	"cyberteam/internal/llm"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// ToolCall represents a tool call from LLM
type ToolCall struct {
	Name string
	Args map[string]interface{}
}

// ToolExecutor interface for executing tools
type ToolExecutor interface {
	ExecuteTool(name string, args map[string]interface{}) (string, error)
}

// Config Agent configuration
type Config struct {
	ID            string
	Name          string
	Role          string
	Model         string
	LLMClient     llm.Client
	MCPExecutor   ToolExecutor // MCP tools executor
	BashExecutor  ToolExecutor // Bash tools executor
	Memory        Memory       // Memory implementation (optional, defaults to FileMemory)
	SystemPrompt  string
	MaxIterations int
	Debug         bool
}

// Agent is the core entity that integrates LLM, MCP tools, Bash tools, and memory
type Agent struct {
	config       Config
	memory       Memory
	toolRegistry *ToolRegistry
}

// Debugf prints a debug message if debug mode is enabled
func (a *Agent) Debugf(format string, args ...interface{}) {
	if a.config.Debug {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}

// New creates a new Agent instance
func New(config Config) *Agent {
	if config.MaxIterations == 0 {
		config.MaxIterations = 5
	}

	// Default to FileMemory if not provided
	memory := config.Memory
	if memory == nil {
		memory = NewFileMemory()
	}

	a := &Agent{
		config:       config,
		memory:       memory,
		toolRegistry: NewToolRegistry(),
	}

	// Register MCP executor if provided
	if config.MCPExecutor != nil {
		a.toolRegistry.RegisterExecutor("mcp", config.MCPExecutor)
	}

	// Register Bash executor if provided
	if config.BashExecutor != nil {
		a.toolRegistry.RegisterExecutor("bash", config.BashExecutor)
	}

	return a
}

// Memory returns the agent's memory
func (a *Agent) Memory() Memory {
	return a.memory
}

// ToolRegistry returns the agent's tool registry
func (a *Agent) ToolRegistry() *ToolRegistry {
	return a.toolRegistry
}

// SetSystemPrompt sets the system prompt
func (a *Agent) SetSystemPrompt(prompt string) {
	a.config.SystemPrompt = prompt
}

// Execute executes a single conversation turn
func (a *Agent) Execute(userPrompt string) string {
	systemMsg := a.buildSystemMessage()

	messages := []llm.Message{
		{Role: "system", Content: systemMsg},
	}

	// Add messages from memory
	messages = append(messages, a.memory.GetMessages()...)

	// Add current user message and save to memory
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: userPrompt,
	})
	a.memory.AddMessage("user", userPrompt)

	// Execute tool calling loop
	return a.executeToolLoop(messages)
}

// ExecuteWithContext executes with conversation context (for meetings/private chats)
func (a *Agent) ExecuteWithContext(transcript, currentMessage string) string {
	systemMsg := a.buildSystemMessage()

	messages := []llm.Message{
		{Role: "system", Content: systemMsg},
	}

	// 从 Memory 加载历史对话
	messages = append(messages, a.memory.GetMessages()...)

	// Parse and add transcript, distinguishing self vs others
	if transcript != "" {
		lines := strings.Split(transcript, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "Boss:") || strings.HasPrefix(line, "Kai:") ||
				strings.HasPrefix(line, "Sarah:") || strings.HasPrefix(line, "Alex:") ||
				strings.HasPrefix(line, "Mia:") {
				role := "user"
				// 如果是自己说的话，标记为 assistant
				if a.config.Name != "" && strings.HasPrefix(line, a.config.Name+":") {
					role = "assistant"
				}
				messages = append(messages, llm.Message{
					Role:    role,
					Content: line,
				})
			}
		}
	}

	// Add current message and save to memory
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: currentMessage,
	})
	a.memory.AddMessage("user", currentMessage)

	// Execute tool calling loop
	return a.executeToolLoop(messages)
}

// executeToolLoop executes the tool calling loop
func (a *Agent) executeToolLoop(messages []llm.Message) string {
	for i := 0; i < a.config.MaxIterations; i++ {
		// 构建工具定义，将工具名转换为合法格式
		var tools []llm.ToolDef
		toolNameMap := make(map[string]string) // LLM工具名 -> 注册名
		for _, t := range a.toolRegistry.ListTools() {
			// 转换: fetch:fetch -> fetch_fetch
			llmName := sanitizeToolName(t.Name)
			toolNameMap[llmName] = t.Name

			// 使用工具的 InputSchema，如果没有则使用默认空对象
			var params any
			if len(t.InputSchema) > 0 {
				if err := json.Unmarshal(t.InputSchema, &params); err != nil {
					params = nil
				}
			}
			if params == nil {
				params = map[string]any{"type": "object", "properties": map[string]any{}}
			}

			tools = append(tools, llm.ToolDef{
				Type: "function",
				Function: struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					Parameters  any    `json:"parameters"`
				}{
					Name:        llmName,
					Description: t.Description,
					Parameters:  params,
				},
			})
		}

		resp, err := a.config.LLMClient.Complete(messages, &llm.CompleteOptions{
			Model:       a.config.Model,
			Temperature: 0.7,
			MaxTokens:   4000,
			Tools:       tools,
		})

		if err != nil {
			a.Debugf("[Agent] LLM 调用失败: %v\n", err)
			return "抱歉，我遇到了一些问题。"
		}

		// 检查是否有 function calling
		hasToolCall := false
		if len(resp.ToolCalls) > 0 {
			hasToolCall = true
			a.Debugf("[Agent] 检测到 %d 个 function calling\n", len(resp.ToolCalls))

			// 先将 assistant 的响应（含 tool_calls）加入消息历史
			messages = append(messages, llm.Message{
				Role:      "assistant",
				Content:   resp.Content,
				ToolCalls: resp.ToolCalls,
			})

			for _, tc := range resp.ToolCalls {
				llmToolName := tc.Function.Name
				toolID := tc.ID

				// 映射回原始工具名
				originalName, ok := toolNameMap[llmToolName]
				if !ok {
					originalName = llmToolName // fallback
				}
				a.Debugf("[Agent] FC 工具: %s -> %s, ID: %s\n", llmToolName, originalName, toolID)

				// 解析参数
				var args map[string]any
				if tc.Function.Arguments != "" {
					json.Unmarshal([]byte(tc.Function.Arguments), &args)
				}

				result, err := a.toolRegistry.Execute(originalName, args)
				if err == nil && result != "" {
					messages = append(messages, llm.Message{
						Role:       "tool",
						ToolCallID: toolID,
						Content:    result,
					})
				} else {
					errMsg := fmt.Sprintf("工具执行失败: %v", err)
					messages = append(messages, llm.Message{
						Role:       "tool",
						ToolCallID: toolID,
						Content:    errMsg,
					})
				}
			}
		} else {
			// 兼容：检查文本格式的工具调用 [TOOL:xxx]{...}
			textToolCalls := ParseToolCalls(resp.Content)
			if len(textToolCalls) > 0 {
				hasToolCall = true
				a.Debugf("[Agent] 检测到 %d 个文本工具调用\n", len(textToolCalls))

				// 先将 assistant 的响应加入消息历史
				messages = append(messages, llm.Message{
					Role:    "assistant",
					Content: resp.Content,
				})

				// 收集所有工具执行结果，用 user 角色发送（避免缺少 tool_call_id 导致 API 错误）
				var resultParts []string
				for _, tc := range textToolCalls {
					// 尝试匹配工具名
					originalName := tc.Name
					for llmName, orig := range toolNameMap {
						if strings.Contains(tc.Name, llmName) || strings.Contains(orig, tc.Name) {
							originalName = orig
							break
						}
					}
					a.Debugf("[Agent] 文本工具: %s -> %s\n", tc.Name, originalName)

					result, err := a.toolRegistry.Execute(originalName, tc.Args)
					if err == nil && result != "" {
						resultParts = append(resultParts, fmt.Sprintf("[%s 执行结果]\n%s", originalName, result))
					} else {
						resultParts = append(resultParts, fmt.Sprintf("[%s 执行失败] %v", originalName, err))
					}
				}
				messages = append(messages, llm.Message{
					Role:    "user",
					Content: strings.Join(resultParts, "\n\n"),
				})
			}
		}

		if hasToolCall {
			continue
		}

		// 没有工具调用
		reply := resp.Content
		a.memory.AddMessage("assistant", reply)
		return reply
	}

	// Max iterations reached - 从当前对话中提取最后一条 assistant 回复
	for j := len(messages) - 1; j >= 0; j-- {
		if messages[j].Role == "assistant" && messages[j].Content != "" {
			a.memory.AddMessage("assistant", messages[j].Content)
			return messages[j].Content
		}
	}

	return "抱歉，需要更多时间来处理这个问题。"
}

// buildSystemMessage builds the system message
func (a *Agent) buildSystemMessage() string {
	var sb strings.Builder
	sb.WriteString(a.config.SystemPrompt)

	// Add tools info
	if a.toolRegistry != nil {
		toolsInfo := a.toolRegistry.GetToolsPrompt()
		if toolsInfo != "" {
			sb.WriteString("\n\n")
			sb.WriteString(toolsInfo)
		}
	}

	return sb.String()
}

// ParseToolCalls parses tool calls from LLM response
// Format: [TOOL:server:tool]{json_args}
func ParseToolCalls(reply string) []ToolCall {
	re := regexp.MustCompile(`\[TOOL:([^\]]+)\]\s*(\{[^\}]+\})`)
	matches := re.FindAllStringSubmatch(reply, -1)

	var toolCalls []ToolCall
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}

		toolName := m[1]
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(m[2]), &args); err != nil {
			continue
		}

		toolCalls = append(toolCalls, ToolCall{
			Name: toolName,
			Args: args,
		})
	}

	return toolCalls
}

// sanitizeToolName 将工具名转换为合法的函数名
// fetch:fetch -> fetch, fetch:fetch_url -> fetch_url
func sanitizeToolName(name string) string {
	// 替换冒号为下划线
	result := strings.ReplaceAll(name, ":", "_")
	// 确保只包含字母、数字、下划线、短横线
	var clean strings.Builder
	for _, c := range result {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			clean.WriteRune(c)
		}
	}
	return clean.String()
}
