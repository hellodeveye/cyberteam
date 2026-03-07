package staffutil

import (
	"cyberteam/internal/llm"
	"encoding/json"
	"fmt"
	"regexp"
)

// ToolCall 表示一个工具调用
type ToolCall struct {
	Name string
	Args map[string]interface{}
}

// ToolExecutor 接口，用于执行工具
type ToolExecutor interface {
	ExecuteTool(name string, args map[string]interface{}) (string, error)
}

// LLMToolCaller 通用 LLM 工具调用器
// 提供多轮工具调用循环能力，所有对话类型都可以使用
type LLMToolCaller struct {
	Client       llm.Client
	Model        string
	ToolExecutor ToolExecutor
	SystemPrompt string
}

// NewLLMToolCaller 创建 LLM 工具调用器
func NewLLMToolCaller(client llm.Client, model string, toolExecutor ToolExecutor, systemPrompt string) *LLMToolCaller {
	return &LLMToolCaller{
		Client:       client,
		Model:        model,
		ToolExecutor: toolExecutor,
		SystemPrompt: systemPrompt,
	}
}

// CompleteWithTools 执行 LLM 调用，支持多轮工具调用
// messages 初始消息列表（通常包含 system 和 user 消息）
// 返回最终回复内容
func (c *LLMToolCaller) CompleteWithTools(messages []llm.Message) string {
	maxIterations := 5

	for i := 0; i < maxIterations; i++ {
		resp, err := c.Client.Complete(messages, &llm.CompleteOptions{
			Model:       c.Model,
			Temperature: 0.7,
			MaxTokens:   500,
		})

		if err != nil {
			return ""
		}

		reply := resp.Content

		// 检查是否有工具调用
		toolCalls := ParseToolCalls(reply)
		if len(toolCalls) > 0 {
			// 执行所有工具
			for _, tc := range toolCalls {
				result, err := c.ToolExecutor.ExecuteTool(tc.Name, tc.Args)
				if err == nil && result != "" {
					messages = append(messages, llm.Message{
						Role:    "tool",
						Content: fmt.Sprintf("[%s] %s", tc.Name, result),
					})
				} else {
					messages = append(messages, llm.Message{
						Role:    "tool",
						Content: fmt.Sprintf("[%s] 执行失败: %v", tc.Name, err),
					})
				}
			}
			continue
		}

		// 无工具调用，返回最终回复
		return reply
	}

	// 达到最大迭代次数
	if len(messages) > 2 {
		return messages[len(messages)-1].Content
	}

	return ""
}

// ParseToolCalls 解析 LLM 回复中的所有工具调用（导出版本）
// 格式: [TOOL:server:tool]{json_args}
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
