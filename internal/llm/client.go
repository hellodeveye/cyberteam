package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client LLM 客户端接口
type Client interface {
	Complete(messages []Message, options *CompleteOptions) (*Response, error)
}

// Message 对话消息
type Message struct {
	Role    string `json:"role"` // system/user/assistant
	Content string `json:"content"`
}

// CompleteOptions 请求选项
type CompleteOptions struct {
	Model       string
	Temperature float64
	MaxTokens   int
	Stream      bool
}

// Response LLM 响应
type Response struct {
	Content      string
	Usage        Usage
	FinishReason string
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

// Complete 发送对话请求
func (c *OpenAIClient) Complete(messages []Message, opts *CompleteOptions) (*Response, error) {
	if opts == nil {
		opts = &CompleteOptions{
			Model:       "deepseek-chat",
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

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.BaseURL+"/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message      Message `json:"message"`
			FinishReason string  `json:"finish_reason"`
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

	return &Response{
		Content:      result.Choices[0].Message.Content,
		FinishReason: result.Choices[0].FinishReason,
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

func (c *MockClient) Complete(messages []Message, opts *CompleteOptions) (*Response, error) {
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
