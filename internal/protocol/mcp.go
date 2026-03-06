package protocol

// MCP 相关消息类型
const (
	MsgMCPCall   MessageType = "mcp_call"   // Staff 请求调用 MCP 工具
	MsgMCPResult MessageType = "mcp_result" // 返回调用结果
	MsgMCPError  MessageType = "mcp_error"  // 调用错误
	MsgMCPList   MessageType = "mcp_list"   // 获取可用工具列表
	MsgMCPNotify MessageType = "mcp_notify" // MCP Server 状态通知
)

// MCPCallRequest MCP 调用请求
type MCPCallRequest struct {
	Server string                 `json:"server,omitempty"` // 可选，指定 Server
	Tool   string                 `json:"tool"`             // 工具名，格式: "server:tool" 或 "tool"
	Args   map[string]interface{} `json:"args"`             // 工具参数
}

// MCPCallResult MCP 调用结果
type MCPCallResult struct {
	Success bool   `json:"success"`
	Result  string `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
}

// MCPListRequest 获取工具列表请求
type MCPListRequest struct {
	Role string `json:"role"` // 请求者角色
}

// MCPListResponse 工具列表响应
type MCPListResponse struct {
	Tools []MCPToolInfo `json:"tools"`
}

// MCPToolInfo 工具信息
type MCPToolInfo struct {
	Name        string `json:"name"`        // 完整工具名，如 "github:search_repos"
	Server      string `json:"server"`      // 所属 Server
	Description string `json:"description"` // 工具描述
}

// MCPNotify MCP 状态通知
type MCPNotify struct {
	Server  string `json:"server"`
	Status  string `json:"status"` // connected, disconnected, error
	Message string `json:"message,omitempty"`
}
