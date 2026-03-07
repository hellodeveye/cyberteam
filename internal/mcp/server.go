package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"cyberteam/internal/tools"
)

// ServerInstance 运行的 MCP Server 实例
type ServerInstance struct {
	Name    string
	Config  Server
	Cmd     *exec.Cmd
	Stdin   io.WriteCloser
	Stdout  *bufio.Reader
	Stderr  io.ReadCloser
	Tools   []Tool
	mu      sync.RWMutex
	ready   bool
	pending map[string]chan *JSONRPCResponse // 等待响应的请求
}

// Tool MCP 工具定义
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// JSONRPCRequest JSON-RPC 请求
type JSONRPCRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      string                 `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// JSONRPCResponse JSON-RPC 响应
type JSONRPCResponse struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      string                 `json:"id,omitempty"`
	Result  map[string]interface{} `json:"result,omitempty"`
	Error   *JSONRPCError          `json:"error,omitempty"`
}

// JSONRPCError JSON-RPC 错误
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewServerInstance 创建 MCP Server 实例
func NewServerInstance(name string, config Server) *ServerInstance {
	return &ServerInstance{
		Name:    name,
		Config:  config,
		pending: make(map[string]chan *JSONRPCResponse),
	}
}

// Start 启动 MCP Server
func (s *ServerInstance) Start() error {
	// 解析命令
	cmdArgs := parseCommand(s.Config.Command)
	if len(cmdArgs) == 0 {
		return fmt.Errorf("empty command")
	}

	// 如果有额外 args，追加到命令后面
	if len(s.Config.Args) > 0 {
		cmdArgs = append(cmdArgs, s.Config.Args...)
	}

	s.Cmd = exec.Command(cmdArgs[0], cmdArgs[1:]...)

	// 设置环境变量
	s.Cmd.Env = os.Environ()
	for k, v := range s.Config.Env {
		s.Cmd.Env = append(s.Cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// 获取管道
	stdin, err := s.Cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	s.Stdin = stdin

	stdout, err := s.Cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	s.Stdout = bufio.NewReader(stdout)

	stderr, err := s.Cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	s.Stderr = stderr

	// 启动进程
	if err := s.Cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}

	// 启动读取循环
	go s.readLoop()

	// 等待初始化完成（获取工具列表）
	if err := s.initialize(); err != nil {
		s.Stop()
		return fmt.Errorf("initialize: %w", err)
	}

	return nil
}

// Stop 停止 MCP Server
func (s *ServerInstance) Stop() error {
	s.mu.Lock()
	s.ready = false
	s.mu.Unlock()

	if s.Cmd != nil && s.Cmd.Process != nil {
		s.Stdin.Close()
		return s.Cmd.Wait()
	}
	return nil
}

// IsReady 是否就绪
func (s *ServerInstance) IsReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ready
}

// initialize 初始化：获取工具列表
func (s *ServerInstance) initialize() error {
	// 发送 initialize 请求
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "init-1",
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]string{
				"name":    "cyberteam-mcp-client",
				"version": "1.0.0",
			},
		},
	}

	respChan := make(chan *JSONRPCResponse, 1)
	s.mu.Lock()
	s.pending["init-1"] = respChan
	s.mu.Unlock()

	if err := s.sendRequest(&req); err != nil {
		return err
	}

	// 等待响应
	select {
	case resp := <-respChan:
		if resp.Error != nil {
			return fmt.Errorf("initialize error: %s", resp.Error.Message)
		}
	case <-time.After(10 * time.Second):
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
func (s *ServerInstance) fetchTools() error {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "tools-1",
		Method:  "tools/list",
	}

	respChan := make(chan *JSONRPCResponse, 1)
	s.mu.Lock()
	s.pending["tools-1"] = respChan
	s.mu.Unlock()

	if err := s.sendRequest(&req); err != nil {
		return err
	}

	select {
	case resp := <-respChan:
		if resp.Error != nil {
			return fmt.Errorf("list tools error: %s", resp.Error.Message)
		}

		// 解析工具列表
		if result, ok := resp.Result["tools"].([]interface{}); ok {
			for _, t := range result {
				if toolMap, ok := t.(map[string]interface{}); ok {
					tool := Tool{
						Name:        getString(toolMap, "name"),
						Description: getString(toolMap, "description"),
					}
					if schema, ok := toolMap["inputSchema"].(map[string]interface{}); ok {
						tool.InputSchema = schema
					}
					s.Tools = append(s.Tools, tool)
				}
			}
		}
		return nil

	case <-time.After(10 * time.Second):
		return fmt.Errorf("list tools timeout")
	}
}

// CallTool 调用工具
func (s *ServerInstance) CallTool(name string, args map[string]interface{}) (*JSONRPCResponse, error) {
	if !s.IsReady() {
		return nil, fmt.Errorf("server not ready")
	}

	reqID := generateID()
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      name,
			"arguments": args,
		},
	}

	respChan := make(chan *JSONRPCResponse, 1)
	s.mu.Lock()
	s.pending[reqID] = respChan
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.pending, reqID)
		s.mu.Unlock()
	}()

	if err := s.sendRequest(&req); err != nil {
		return nil, err
	}

	select {
	case resp := <-respChan:
		return resp, nil
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("call tool timeout")
	}
}

// sendRequest 发送请求
func (s *ServerInstance) sendRequest(req *JSONRPCRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err = fmt.Fprintln(s.Stdin, string(data))
	return err
}

// readLoop 读取响应循环
func (s *ServerInstance) readLoop() {
	for {
		line, err := s.Stdout.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(os.Stderr, "[MCP:%s] read error: %v\n", s.Name, err)
			}
			return
		}

		var resp JSONRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}

		// 找到等待的 channel
		s.mu.Lock()
		ch, ok := s.pending[resp.ID]
		s.mu.Unlock()

		if ok {
			ch <- &resp
		}
	}
}

// parseCommand 解析命令字符串
func parseCommand(cmd string) []string {
	// 简单处理，支持空格分隔
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

// generateID 生成唯一 ID
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// BashServer 内置 Bash MCP Server（轻量包装）
type BashServer struct {
	name     string
	bashTool *tools.BashTool
	mu       sync.RWMutex
	ready    bool
	pending  map[string]chan *JSONRPCResponse
}

// NewBashServer 创建 Bash MCP Server
func NewBashServer(name string, bashTool *tools.BashTool) *BashServer {
	return &BashServer{
		name:     name,
		bashTool: bashTool,
		ready:    true,
		pending:  make(map[string]chan *JSONRPCResponse),
	}
}

// Name 返回 Server 名称
func (s *BashServer) Name() string {
	return s.name
}

// Start 启动 Server（内置 Server 不需要启动）
func (s *BashServer) Start() error {
	return nil
}

// Stop 停止 Server
func (s *BashServer) Stop() error {
	return nil
}

// IsReady 是否就绪
func (s *BashServer) IsReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ready
}

// Tools 返回工具列表
func (s *BashServer) Tools() []Tool {
	return []Tool{
		{
			Name:        "execute",
			Description: "执行 shell 命令（基于 PROFILE.md 中的 allow/deny 列表进行安全检查）",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "要执行的命令",
					},
				},
				"required": []string{"command"},
			},
		},
	}
}

// CallTool 调用工具
func (s *BashServer) CallTool(name string, args map[string]interface{}) (*JSONRPCResponse, error) {
	if name != "execute" {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &JSONRPCError{Code: -32601, Message: "unknown method"},
		}, nil
	}

	cmd, _ := args["command"].(string)
	result := s.bashTool.Execute(cmd)

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": result.Output,
				},
			},
			"success": result.Success,
			"error":   result.Error,
		},
	}, nil
}
