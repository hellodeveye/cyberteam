package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// BashTool 安全的 Bash 执行工具
type BashTool struct {
	mu sync.RWMutex

	// 配置
	workDir   string        // 允许的工作目录
	timeout   time.Duration // 命令超时
	maxOutput int           // 最大输出字节

	// 白名单
	allowedCmds map[string]bool // 允许的命令
	blockedCmds map[string]bool // 禁止的命令

	// 审计
	history []CommandRecord
}

// CommandRecord 命令执行记录
type CommandRecord struct {
	Time     time.Time `json:"time"`
	Command  string    `json:"command"`
	WorkDir  string    `json:"work_dir"`
	Success  bool      `json:"success"`
	Output   string    `json:"output,omitempty"`
	Error    string    `json:"error,omitempty"`
	Duration int64     `json:"duration_ms"`
}

// Result 执行结果
type Result struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

// NewBashTool 创建 Bash 工具
func NewBashTool(workDir string) *BashTool {
	return &BashTool{
		workDir:     workDir,
		timeout:     30 * time.Second,
		maxOutput:   1024 * 1024, // 1MB
		allowedCmds: defaultAllowedCommands(),
		blockedCmds: defaultBlockedCommands(),
		history:     make([]CommandRecord, 0),
	}
}

// NewBashToolWithLists 使用自定义命令白名单和黑名单创建 BashTool
func NewBashToolWithLists(workDir string, allow, deny []string) *BashTool {
	bt := &BashTool{
		workDir:   workDir,
		timeout:   30 * time.Second,
		maxOutput: 1024 * 1024,
		history:   make([]CommandRecord, 0),
	}
	if len(allow) > 0 {
		bt.allowedCmds = make(map[string]bool, len(allow))
		for _, cmd := range allow {
			bt.allowedCmds[cmd] = true
		}
	} else {
		bt.allowedCmds = defaultAllowedCommands()
	}
	bt.blockedCmds = defaultBlockedCommands()
	for _, cmd := range deny {
		bt.blockedCmds[cmd] = true
	}
	return bt
}

// Execute 执行命令（安全检查）
func (b *BashTool) Execute(command string) *Result {
	return b.ExecuteInDir(b.workDir, command)
}

// ExecuteInDir 在指定目录执行命令
func (b *BashTool) ExecuteInDir(workDir, command string) *Result {
	start := time.Now()

	// 1. 命令解析和校验
	cmd, args, err := b.parseCommand(command)
	if err != nil {
		return &Result{Success: false, Error: err.Error()}
	}

	// 2. 安全检查
	if err := b.validateCommand(cmd, args); err != nil {
		return &Result{Success: false, Error: err.Error()}
	}

	// 3. 工作目录检查（防止目录遍历）
	if !b.isSafePath(workDir) {
		return &Result{Success: false, Error: "invalid work directory"}
	}

	// 4. 确保目录存在
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return &Result{Success: false, Error: fmt.Sprintf("create dir: %v", err)}
	}

	// 5. 执行命令（带超时）
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()

	execCmd := exec.CommandContext(ctx, cmd, args...)
	execCmd.Dir = workDir
	execCmd.Env = b.sanitizedEnv()

	output, err := execCmd.CombinedOutput()

	// 6. 截断输出
	result := &Result{Success: err == nil}
	if len(output) > b.maxOutput {
		result.Output = string(output[:b.maxOutput]) + "\n... (truncated)"
	} else {
		result.Output = string(output)
	}
	if err != nil {
		result.Error = err.Error()
	}

	// 7. 记录审计日志
	record := CommandRecord{
		Time:     time.Now(),
		Command:  command,
		WorkDir:  workDir,
		Success:  result.Success,
		Output:   truncate(result.Output, 1000),
		Error:    result.Error,
		Duration: time.Since(start).Milliseconds(),
	}
	b.recordHistory(record)

	return result
}

// ExecuteScript 执行多行脚本
func (b *BashTool) ExecuteScript(script string) *Result {
	// 逐行执行，遇到错误停止
	lines := strings.Split(script, "\n")
	var outputs []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		result := b.Execute(line)
		outputs = append(outputs, fmt.Sprintf("$ %s\n%s", line, result.Output))

		if !result.Success {
			outputs = append(outputs, fmt.Sprintf("Error: %s", result.Error))
			return &Result{
				Success: false,
				Output:  strings.Join(outputs, "\n"),
				Error:   result.Error,
			}
		}
	}

	return &Result{
		Success: true,
		Output:  strings.Join(outputs, "\n"),
	}
}

// parseCommand 解析命令
func (b *BashTool) parseCommand(command string) (string, []string, error) {
	// 移除危险字符但保留基本 shell 功能
	command = sanitizeCommand(command)

	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", nil, fmt.Errorf("empty command")
	}

	return parts[0], parts[1:], nil
}

// validateCommand 验证命令安全
func (b *BashTool) validateCommand(cmd string, args []string) error {
	// 检查禁止命令（黑名单）
	if b.blockedCmds[cmd] {
		return fmt.Errorf("command '%s' is blocked", cmd)
	}

	// 检查允许命令（白名单）
	if len(b.allowedCmds) > 0 && !b.allowedCmds[cmd] {
		return fmt.Errorf("command '%s' is not allowed", cmd)
	}

	// 检查参数中的危险模式
	for _, arg := range args {
		if containsDangerousPattern(arg) {
			return fmt.Errorf("dangerous pattern in argument: %s", arg)
		}
	}

	// 如果是绝对路径，检查是否在允许范围内
	if strings.HasPrefix(cmd, "/") {
		if !b.isAllowedPath(cmd) {
			return fmt.Errorf("command path not allowed: %s", cmd)
		}
	}

	return nil
}

// isSafePath 检查路径安全
func (b *BashTool) isSafePath(path string) bool {
	// 解析绝对路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// 必须在 workDir 下
	workDirAbs, _ := filepath.Abs(b.workDir)
	return strings.HasPrefix(absPath, workDirAbs)
}

// isAllowedPath 检查命令路径
func (b *BashTool) isAllowedPath(cmd string) bool {
	allowedPrefixes := []string{
		"/usr/bin/",
		"/bin/",
		"/usr/local/bin/",
	}
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(cmd, prefix) {
			return true
		}
	}
	return false
}

// sanitizedEnv 清理环境变量
func (b *BashTool) sanitizedEnv() []string {
	// 只保留必要的环境变量
	allowed := map[string]bool{
		"PATH":     true,
		"HOME":     true,
		"GOPATH":   true,
		"GOROOT":   true,
		"NODE_ENV": true,
	}

	var env []string
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if allowed[parts[0]] {
			env = append(env, e)
		}
	}
	return env
}

// recordHistory 记录历史
func (b *BashTool) recordHistory(record CommandRecord) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.history = append(b.history, record)

	// 限制历史大小
	if len(b.history) > 1000 {
		b.history = b.history[len(b.history)-500:]
	}
}

// GetHistory 获取执行历史
func (b *BashTool) GetHistory(limit int) []CommandRecord {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if limit <= 0 || limit > len(b.history) {
		limit = len(b.history)
	}

	start := len(b.history) - limit
	if start < 0 {
		start = 0
	}

	return b.history[start:]
}

// WriteFile 安全写入文件
func (b *BashTool) WriteFile(relativePath string, content []byte) *Result {
	// 防止目录遍历
	if strings.Contains(relativePath, "..") {
		return &Result{Success: false, Error: "invalid path"}
	}

	fullPath := filepath.Join(b.workDir, relativePath)

	// 确保目录存在
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &Result{Success: false, Error: err.Error()}
	}

	// 写入文件
	if err := os.WriteFile(fullPath, content, 0644); err != nil {
		return &Result{Success: false, Error: err.Error()}
	}

	return &Result{Success: true, Output: fmt.Sprintf("written: %s", relativePath)}
}

// ReadFile 安全读取文件
func (b *BashTool) ReadFile(relativePath string) *Result {
	if strings.Contains(relativePath, "..") {
		return &Result{Success: false, Error: "invalid path"}
	}

	fullPath := filepath.Join(b.workDir, relativePath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return &Result{Success: false, Error: err.Error()}
	}

	return &Result{Success: true, Output: string(content)}
}

// 辅助函数

func defaultAllowedCommands() map[string]bool {
	return map[string]bool{
		// Go 开发
		"go":        true,
		"gofmt":     true,
		"golint":    true,
		"goimports": true,

		// Python 开发
		"python":  true,
		"python3": true,
		"pip":     true,
		"pytest":  true,

		// Node.js
		"node": true,
		"npm":  true,
		"yarn": true,
		"npx":  true,

		// 版本控制
		"git": true,

		// 文件操作
		"ls":    true,
		"cat":   true,
		"head":  true,
		"tail":  true,
		"grep":  true,
		"find":  true,
		"mkdir": true,
		"touch": true,
		"rm":    true,
		"cp":    true,
		"mv":    true,
		"echo":  true,
		"pwd":   true,

		// 构建工具
		"make":   true,
		"docker": true,

		// 测试
		"curl": true,
		"wget": true,
	}
}

func defaultBlockedCommands() map[string]bool {
	return map[string]bool{
		// 危险命令
		"sudo":   true,
		"su":     true,
		"passwd": true,
		"chmod":  true,
		"chown":  true,

		// 网络危险
		"nc":     true,
		"netcat": true,
		"telnet": true,
		"ssh":    true,
		"scp":    true,
		"sftp":   true,

		// 系统危险
		"reboot":   true,
		"shutdown": true,
		"halt":     true,
		"poweroff": true,
		"mkfs":     true,
		"fdisk":    true,
		"dd":       true,

		// 其他危险
		":(){:|:&};:": true, // fork bomb
	}
}

func sanitizeCommand(cmd string) string {
	// 完全禁止危险的 shell 元字符，防止命令注入攻击
	dangerous := []string{";", "|", "&", ">", "<", "`", "$", "(", ")", "{", "}", "[", "]", "\\", "!", "#", "*", "?", "~"}
	for _, d := range dangerous {
		cmd = strings.ReplaceAll(cmd, d, "")
	}
	return strings.TrimSpace(cmd)
}

func containsDangerousPattern(arg string) bool {
	dangerous := []string{
		"../",
		"..\\",
		"/etc/passwd",
		"/etc/shadow",
		"~/.ssh",
		"/.bashrc",
		"/.zshrc",
		"rm -rf /",
		":(){:|:&};:",
	}

	for _, d := range dangerous {
		if strings.Contains(arg, d) {
			return true
		}
	}
	return false
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
