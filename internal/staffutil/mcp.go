package staffutil

import (
	"encoding/json"
	"fmt"
	"strings"
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

// BashResult Bash 命令执行结果
type BashResult struct {
	Success bool
	Output  string
	Error   string
}

// ExecuteBash 通过 MCP 执行 bash 命令
func (c *MCPClient) ExecuteBash(command, workDir string) *BashResult {
	args := map[string]interface{}{
		"command": command,
	}
	if workDir != "" {
		args["work_dir"] = workDir
	}
	return c.callBashTool("bash:execute", args)
}

// WriteBashFile 通过 MCP 写入文件
func (c *MCPClient) WriteBashFile(path string, content []byte, workDir string) *BashResult {
	args := map[string]interface{}{
		"path":    path,
		"content": string(content),
	}
	if workDir != "" {
		args["work_dir"] = workDir
	}
	return c.callBashTool("bash:write_file", args)
}

// callBashTool 调用 bash 类工具并解析结果
func (c *MCPClient) callBashTool(tool string, args map[string]interface{}) *BashResult {
	req := protocol.Message{
		Type: protocol.MsgMCPCall,
		ID:   fmt.Sprintf("mcp-%d", time.Now().UnixNano()),
		Payload: map[string]interface{}{
			"tool": tool,
			"args": args,
		},
	}

	resp, err := c.requestFunc(req)
	if err != nil {
		return &BashResult{Success: false, Error: err.Error()}
	}

	if resp.Type == protocol.MsgMCPError {
		errMsg, _ := resp.Payload["error"].(string)
		return &BashResult{Success: false, Error: errMsg}
	}

	success, _ := resp.Payload["success"].(bool)
	result, _ := resp.Payload["result"].(string)
	errMsg, _ := resp.Payload["error"].(string)

	return &BashResult{
		Success: success,
		Output:  result,
		Error:   errMsg,
	}
}

// CallTool 调用 MCP 工具
func (c *MCPClient) CallTool(tool string, args map[string]interface{}) (string, error) {
	req := protocol.Message{
		Type: protocol.MsgMCPCall,
		ID:   fmt.Sprintf("mcp-%d", time.Now().UnixNano()),
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
		ID:   fmt.Sprintf("mcp-%d", time.Now().UnixNano()),
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

// GetToolsPrompt 获取工具提示（用于 System Prompt）
func (c *MCPClient) GetToolsPrompt() string {
	tools, err := c.ListTools()
	if err != nil || len(tools) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "**可用工具：**")
	for _, t := range tools {
		lines = append(lines, fmt.Sprintf("- %s: %s", t.Name, t.Description))
		// 展示参数列表（从 InputSchema.properties 提取）
		if params := extractSchemaParams(t.InputSchema); len(params) > 0 {
			lines = append(lines, fmt.Sprintf("  参数: %s", strings.Join(params, ", ")))
		}
	}
	lines = append(lines, "")
	lines = append(lines, "**使用工具：**")
	lines = append(lines, "当你需要外部数据时，可以调用工具。在回复中用 [TOOL:tool_name]json_args 格式")
	lines = append(lines, `例如：[TOOL:fetch:fetch_url]{"url":"https://example.com"}`)

	return strings.Join(lines, "\n")
}

// extractSchemaParams 从 JSON Schema 提取参数名和类型描述
// 格式: ["url(string,required)", "timeout(integer)"]
func extractSchemaParams(schema map[string]interface{}) []string {
	if schema == nil {
		return nil
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok || len(props) == 0 {
		return nil
	}

	// 收集 required 集合
	required := map[string]bool{}
	if req, ok := schema["required"].([]interface{}); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				required[s] = true
			}
		}
	}

	var params []string
	for name, def := range props {
		typ := "any"
		if defMap, ok := def.(map[string]interface{}); ok {
			if t, ok := defMap["type"].(string); ok {
				typ = t
			} else if desc, ok := defMap["description"].(string); ok && desc != "" {
				// 无 type 时用 description 片段作提示
				if len(desc) > 20 {
					desc = desc[:20]
				}
				typ = desc
			}
		}
		if required[name] {
			params = append(params, fmt.Sprintf("%s(%s,required)", name, typ))
		} else {
			params = append(params, fmt.Sprintf("%s(%s)", name, typ))
		}
	}
	return params
}
