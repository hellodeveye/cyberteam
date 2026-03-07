package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Client LLM 客户端接口
type Client interface {
	Complete(ctx context.Context, messages []Message, options *CompleteOptions) (*Response, error)
}

// Message 对话消息
type Message struct {
	Role       string     `json:"role"` // system/user/assistant/tool
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // Function calling 所需
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // Assistant 消息中的工具调用
}

// CompleteOptions 请求选项
type CompleteOptions struct {
	Model       string
	Temperature float64
	MaxTokens   int
	Stream      bool
	Tools       []ToolDef // Function calling 工具定义
}

// ToolDef 函数调用工具定义
type ToolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Response LLM 响应
type Response struct {
	Content      string
	Usage        Usage
	FinishReason string
	ToolCalls    []ToolCall // Function calling 返回的工具调用
}

// Usage Token 使用情况
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// OpenAIClient OpenAI 兼容客户端
type OpenAIClient struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// NewOpenAIClient 创建客户端
func NewOpenAIClient(apiKey, baseURL string) *OpenAIClient {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIClient{
		APIKey:  apiKey,
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// getDefaultModel 从环境变量获取默认模型
func getDefaultModel() string {
	if model := os.Getenv("OPENAI_MODEL"); model != "" {
		return model
	}
	if model := os.Getenv("DEEPSEEK_MODEL"); model != "" {
		return model
	}
	return "deepseek-chat"
}

// Complete 发送对话请求
func (c *OpenAIClient) Complete(ctx context.Context, messages []Message, opts *CompleteOptions) (*Response, error) {
	if opts == nil {
		opts = &CompleteOptions{
			Model:       getDefaultModel(),
			Temperature: 0.7,
			MaxTokens:   2000,
		}
	}

	reqBody := map[string]any{
		"model":       opts.Model,
		"messages":    messages,
		"temperature": opts.Temperature,
		"max_tokens":  opts.MaxTokens,
		"stream":      false,
	}

	// 添加 function calling 工具
	if len(opts.Tools) > 0 {
		reqBody["tools"] = opts.Tools
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// 带指数退避的重试逻辑（最多重试 3 次，仅对暂时性错误重试）
	const maxRetries = 3
	var body []byte
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second // 1s, 2s, 4s
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewReader(jsonData))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.APIKey)

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("do request: %w", err)
			continue // 网络错误，重试
		}

		body, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read body: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			lastErr = nil
			break
		}

		// 5xx 或 429 限流可重试，其他错误直接返回
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
			continue
		}
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	if lastErr != nil {
		return nil, lastErr
	}

	var result struct {
		Choices []struct {
			Message struct {
				Role      string     `json:"role"`
				Content   string     `json:"content"`
				ToolCalls []ToolCall `json:"tool_calls,omitempty"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	// 提取 tool_calls（从 message 内部）
	var toolCalls []ToolCall
	if len(result.Choices[0].Message.ToolCalls) > 0 {
		toolCalls = result.Choices[0].Message.ToolCalls
	}

	return &Response{
		Content:      result.Choices[0].Message.Content,
		FinishReason: result.Choices[0].FinishReason,
		ToolCalls:    toolCalls,
		Usage: Usage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		},
	}, nil
}

// MockClient 模拟客户端（用于测试）
type MockClient struct {
	Responses []string
	Index     int
}

func (c *MockClient) Complete(ctx context.Context, messages []Message, opts *CompleteOptions) (*Response, error) {
	content := "模拟响应：已根据您的需求生成了内容。"
	if c.Index < len(c.Responses) {
		content = c.Responses[c.Index]
		c.Index++
	}
	return &Response{
		Content:      content,
		FinishReason: "stop",
		Usage:        Usage{TotalTokens: 100},
	}, nil
}
