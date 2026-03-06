package staffutil

import (
	"encoding/json"
	"fmt"
	"time"

	"cyberteam/internal/protocol"
)

// MCPClient Staff 端的 MCP 客户端
type MCPClient struct {
	requestFunc func(msg protocol.Message) (*protocol.Message, error)
}

// NewMCPClient 创建 MCP 客户端
// requestFunc 是发送请求并等待响应的函数
func NewMCPClient(requestFunc func(msg protocol.Message) (*protocol.Message, error)) *MCPClient {
	return &MCPClient{requestFunc: requestFunc}
}

// CallTool 调用 MCP 工具
func (c *MCPClient) CallTool(tool string, args map[string]interface{}) (string, error) {
	req := protocol.Message{
		Type: protocol.MsgMCPCall,
		ID:   generateMCPID(),
		Payload: map[string]interface{}{
			"tool": tool,
			"args": args,
		},
	}

	resp, err := c.requestFunc(req)
	if err != nil {
		return "", err
	}

	if resp.Type == protocol.MsgMCPError {
		errMsg, _ := resp.Payload["error"].(string)
		return "", fmt.Errorf("mcp error: %s", errMsg)
	}

	result, _ := resp.Payload["result"].(string)
	return result, nil
}

// ListTools 获取可用工具列表
func (c *MCPClient) ListTools() ([]protocol.MCPToolInfo, error) {
	req := protocol.Message{
		Type: protocol.MsgMCPList,
		ID:   generateMCPID(),
		Payload: map[string]interface{}{
			"role": "", // Boss 会根据 Staff 角色自动判断
		},
	}

	resp, err := c.requestFunc(req)
	if err != nil {
		return nil, err
	}

	if resp.Type == protocol.MsgMCPError {
		errMsg, _ := resp.Payload["error"].(string)
		return nil, fmt.Errorf("mcp error: %s", errMsg)
	}

	toolsData, _ := json.Marshal(resp.Payload["tools"])
	var tools []protocol.MCPToolInfo
	if err := json.Unmarshal(toolsData, &tools); err != nil {
		return nil, err
	}

	return tools, nil
}

// generateMCPID 生成简单 ID
func generateMCPID() string {
	return fmt.Sprintf("mcp-%d", time.Now().UnixNano())
}
