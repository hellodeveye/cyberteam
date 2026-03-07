package staffutil

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"cyberteam/internal/mcp"
	"cyberteam/internal/protocol"
)

// MCP 超时常量
const (
	MCPInitTimeout    = 10 * time.Second // 初始化超时
	MCPToolListTimeout = 10 * time.Second // 获取工具列表超时
	MCPToolCallTimeout = 30 * time.Second // 工具调用超时
)

// StaffMCPClient Staff 直接连接的 MCP 客户端
type StaffMCPClient struct {
	role    string
	servers map[string]*staffMCPServer
	mu      sync.RWMutex
	tools   []protocol.MCPToolInfo
	toolsMu sync.RWMutex
}

// staffMCPServer Staff 端的 MCP Server 实例
type staffMCPServer struct {
	name    string
	config  mcp.Server
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	stderr  io.ReadCloser
	tools   []mcp.Tool
	mu      sync.RWMutex
	ready   bool
	pending map[string]chan *jsonRPCResponse
}

// jsonRPCRequest JSON-RPC 请求
type jsonRPCRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      string                 `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// jsonRPCResponse JSON-RPC 响应
type jsonRPCResponse struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      string                 `json:"id,omitempty"`
	Result  map[string]interface{} `json:"result,omitempty"`
	Error   *jsonRPCError          `json:"error,omitempty"`
}

// jsonRPCError JSON-RPC 错误
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewStaffMCPClient 创建 Staff MCP 客户端
// configPath: MCP 配置文件路径
// role: Staff 的角色 (developer, product, tester)
func NewStaffMCPClient(configPath, role string) (*StaffMCPClient, error) {
	client := &StaffMCPClient{
		role:    role,
		servers: make(map[string]*staffMCPServer),
	}

	// 加载配置
	config, err := mcp.LoadConfig(configPath)
	if err != nil {
		// 如果配置文件不存在，使用空配置
		config = &mcp.Config{
			Servers: make(map[string]mcp.Server),
		}
	}

	// 启动允许该角色的 MCP Server
	enabled := config.GetEnabledServers()

	for name, serverConfig := range enabled {
		// 检查角色权限 - 简单检查 role 是否在 ACL roles 列表中
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

		server := &staffMCPServer{
			name:    name,
			config:  serverConfig,
			pending: make(map[string]chan *jsonRPCResponse),
		}

		if err := server.start(); err != nil {
			continue
		}

		client.servers[name] = server
	}

	// 收集工具列表
	client.refreshTools()

	return client, nil
}

// refreshTools 刷新工具列表
func (c *StaffMCPClient) refreshTools() {
	c.toolsMu.Lock()
	defer c.toolsMu.Unlock()

	var tools []protocol.MCPToolInfo
	for serverName, server := range c.servers {
		server.mu.RLock()
		// 检查角色是否允许访问该服务器（服务器级别权限检查）
		roleAllowed := false
		for _, r := range server.config.ACL.Roles {
			if r == c.role {
				roleAllowed = true
				break
			}
		}
		if !roleAllowed {
			server.mu.RUnlock()
			continue
		}

		for _, tool := range server.tools {
			tools = append(tools, protocol.MCPToolInfo{
				Name:        fmt.Sprintf("%s:%s", serverName, tool.Name),
				Server:      serverName,
				Description: tool.Description,
				InputSchema: tool.InputSchema,
			})
		}
		server.mu.RUnlock()
	}
	c.tools = tools
}

// isToolAllowed 检查工具是否允许使用
func (c *StaffMCPClient) isToolAllowed(serverName, toolName string) bool {
	server, ok := c.servers[serverName]
	if !ok {
		return false
	}
	return server.config.IsToolAllowed(toolName, c.role)
}

// ListTools 获取可用工具列表
func (c *StaffMCPClient) ListTools() []protocol.MCPToolInfo {
	c.toolsMu.RLock()
	defer c.toolsMu.RUnlock()
	return c.tools
}

// CallTool 调用 MCP 工具
func (c *StaffMCPClient) CallTool(fullToolName string, args map[string]interface{}) (string, error) {
	// 解析工具名，格式: "server:tool" 或 "tool"
	parts := strings.SplitN(fullToolName, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid tool name format: %s (expected server:tool)", fullToolName)
	}

	serverName := parts[0]
	toolName := parts[1]

	c.mu.RLock()
	server, ok := c.servers[serverName]
	c.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("server not found: %s", serverName)
	}

	// 权限检查
	if !server.config.IsToolAllowed(toolName, c.role) {
		return "", fmt.Errorf("permission denied: %s cannot use %s", c.role, fullToolName)
	}

	return server.callTool(toolName, args)
}

// GetToolsPrompt 获取工具提示（用于 System Prompt）
func (c *StaffMCPClient) GetToolsPrompt() string {
	tools := c.ListTools()
	if len(tools) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "**可用工具（通过 function calling 调用）：**")
	for _, t := range tools {
		lines = append(lines, fmt.Sprintf("- %s: %s", t.Name, t.Description))
		// 展示参数列表（从 InputSchema.properties 提取）
		if params := extractSchemaParams(t.InputSchema); len(params) > 0 {
			lines = append(lines, fmt.Sprintf("  参数: %s", strings.Join(params, ", ")))
		}
	}

	return strings.Join(lines, "\n")
}

// Stop 停止所有 MCP Server
func (c *StaffMCPClient) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, server := range c.servers {
		server.stop()
	}
	c.servers = make(map[string]*staffMCPServer)
}

// start 启动 MCP Server
func (s *staffMCPServer) start() error {
	// 解析命令
	cmdArgs := parseCommand(s.config.Command)
	if len(cmdArgs) == 0 {
		return fmt.Errorf("empty command")
	}

	// 如果有额外 args，追加到命令后面
	if len(s.config.Args) > 0 {
		cmdArgs = append(cmdArgs, s.config.Args...)
	}

	s.cmd = exec.Command(cmdArgs[0], cmdArgs[1:]...)

	// 设置环境变量
	s.cmd.Env = os.Environ()
	for k, v := range s.config.Env {
		s.cmd.Env = append(s.cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// 获取管道
	stdin, err := s.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	s.stdin = stdin

	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	s.stdout = bufio.NewReader(stdout)

	stderr, err := s.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	s.stderr = stderr

	// 启动进程
	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}

	// 启动读取循环
	go s.readLoop()

	// 等待初始化完成（获取工具列表）
	if err := s.initialize(); err != nil {
		s.stop()
		return fmt.Errorf("initialize: %w", err)
	}

	return nil
}

// stop 停止 MCP Server
func (s *staffMCPServer) stop() error {
	s.mu.Lock()
	s.ready = false
	s.mu.Unlock()

	if s.cmd != nil && s.cmd.Process != nil {
		s.stdin.Close()
		return s.cmd.Wait()
	}
	return nil
}

// initialize 初始化：获取工具列表
func (s *staffMCPServer) initialize() error {
	// 发送 initialize 请求
	reqID := "init-1"
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]string{
				"name":    "cyberteam-staff-mcp",
				"version": "1.0.0",
			},
		},
	}

	respChan := make(chan *jsonRPCResponse, 1)
	s.mu.Lock()
	s.pending[reqID] = respChan
	s.mu.Unlock()

	// 确保超时时清理 pending channel，防止 goroutine 泄漏
	defer func() {
		s.mu.Lock()
		delete(s.pending, reqID)
		s.mu.Unlock()
	}()

	if err := s.sendRequest(&req); err != nil {
		return err
	}

	// 等待响应
	select {
	case resp, ok := <-respChan:
		if !ok {
			return fmt.Errorf("server disconnected during initialize")
		}
		if resp.Error != nil {
			return fmt.Errorf("initialize error: %s", resp.Error.Message)
		}
	case <-time.After(MCPInitTimeout):
		return fmt.Errorf("initialize timeout")
	}

	// 获取工具列表
	if err := s.fetchTools(); err != nil {
		return fmt.Errorf("fetch tools: %w", err)
	}

	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()

	return nil
}

// fetchTools 获取工具列表
func (s *staffMCPServer) fetchTools() error {
	reqID := "tools-1"
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  "tools/list",
	}

	respChan := make(chan *jsonRPCResponse, 1)
	s.mu.Lock()
	s.pending[reqID] = respChan
	s.mu.Unlock()

	// 确保超时时清理 pending channel
	defer func() {
		s.mu.Lock()
		delete(s.pending, reqID)
		s.mu.Unlock()
	}()

	if err := s.sendRequest(&req); err != nil {
		return err
	}

	select {
	case resp, ok := <-respChan:
		if !ok {
			return fmt.Errorf("server disconnected during fetchTools")
		}
		if resp.Error != nil {
			return fmt.Errorf("list tools error: %s", resp.Error.Message)
		}

		// 解析工具列表
		if result, ok := resp.Result["tools"].([]interface{}); ok {
			for _, t := range result {
				if toolMap, ok := t.(map[string]interface{}); ok {
					toolName := getString(toolMap, "name")
					toolDesc := getString(toolMap, "description")
					tool := mcp.Tool{
						Name:        toolName,
						Description: toolDesc,
					}
					if schema, ok := toolMap["inputSchema"].(map[string]interface{}); ok {
						tool.InputSchema = schema
					}
					s.tools = append(s.tools, tool)
				}
			}
		}
		return nil

	case <-time.After(MCPToolListTimeout):
		return fmt.Errorf("list tools timeout")
	}
}

// callTool 调用工具
func (s *staffMCPServer) callTool(name string, args map[string]interface{}) (string, error) {
	s.mu.RLock()
	ready := s.ready
	s.mu.RUnlock()

	if !ready {
		return "", fmt.Errorf("server not ready")
	}

	reqID := generateID()
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      name,
			"arguments": args,
		},
	}

	respChan := make(chan *jsonRPCResponse, 1)
	s.mu.Lock()
	s.pending[reqID] = respChan
	s.mu.Unlock()

	// 确保超时时清理 pending channel（delete 对已删除的 key 是安全的）
	defer func() {
		s.mu.Lock()
		delete(s.pending, reqID)
		s.mu.Unlock()
	}()

	if err := s.sendRequest(&req); err != nil {
		return "", err
	}

	select {
	case resp, ok := <-respChan:
		if !ok {
			return "", fmt.Errorf("server disconnected")
		}
		if resp.Error != nil {
			return "", fmt.Errorf("tool error: %s", resp.Error.Message)
		}
		// 解析结果
		return parseToolResult(resp.Result), nil

	case <-time.After(MCPToolCallTimeout):
		return "", fmt.Errorf("call tool timeout")
	}
}

// sendRequest 发送请求
func (s *staffMCPServer) sendRequest(req *jsonRPCRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err = fmt.Fprintln(s.stdin, string(data))
	return err
}

// readLoop 读取响应循环
func (s *staffMCPServer) readLoop() {
	defer func() {
		// readLoop 退出时，关闭所有等待中的 pending channels，防止 goroutine 泄漏
		s.mu.Lock()
		for id, ch := range s.pending {
			close(ch)
			delete(s.pending, id)
		}
		s.mu.Unlock()
	}()

	for {
		line, err := s.stdout.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(os.Stderr, "[StaffMCP:%s] readLoop error: %v\n", s.name, err)
			}
			return
		}

		var resp jsonRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}

		// 找到等待的 channel，发送后立即从 pending 中删除
		s.mu.Lock()
		ch, ok := s.pending[resp.ID]
		if ok {
			delete(s.pending, resp.ID)
		}
		s.mu.Unlock()

		if ok {
			ch <- &resp
		}
	}
}

// parseCommand 解析命令字符串
func parseCommand(cmd string) []string {
	var args []string
	var current string
	var inQuote bool
	var quoteChar rune

	for _, r := range cmd {
		switch r {
		case '"', '\'':
			if !inQuote {
				inQuote = true
				quoteChar = r
			} else if quoteChar == r {
				inQuote = false
				quoteChar = 0
			} else {
				current += string(r)
			}
		case ' ', '\t':
			if inQuote {
				current += string(r)
			} else if current != "" {
				args = append(args, current)
				current = ""
			}
		default:
			current += string(r)
		}
	}

	if current != "" {
		args = append(args, current)
	}

	return args
}

// getString 从 map 获取字符串
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// parseToolResult 解析工具返回结果
func parseToolResult(result map[string]interface{}) string {
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

// generateID 生成唯一 ID
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
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
