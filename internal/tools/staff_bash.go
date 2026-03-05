package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

// StaffBashTool 为 Staff 提供的 Bash 工具封装
type StaffBashTool struct {
	bash    *BashTool
	project string
	stage   string
}

// NewStaffBashTool 创建 Staff 专用的 Bash 工具
func NewStaffBashTool(workspacesDir, projectName, projectID, stage string) *StaffBashTool {
	// 构建工作目录: workspaces/ProjectName-ID/04-develop/
	projectDir := filepath.Join(workspacesDir, 
		sanitize(projectName)+"-"+projectID[:8],
		stage)
	
	return &StaffBashTool{
		bash:    NewBashTool(projectDir),
		project: projectName,
		stage:   stage,
	}
}

// WriteCodeFile 写入代码文件
func (s *StaffBashTool) WriteCodeFile(filePath string, code string) error {
	result := s.bash.WriteFile(filePath, []byte(code))
	if !result.Success {
		return fmt.Errorf("write file: %s", result.Error)
	}
	return nil
}

// ReadCodeFile 读取代码文件
func (s *StaffBashTool) ReadCodeFile(filePath string) (string, error) {
	result := s.bash.ReadFile(filePath)
	if !result.Success {
		return "", fmt.Errorf("read file: %s", result.Error)
	}
	return result.Output, nil
}

// Execute 执行命令
func (s *StaffBashTool) Execute(command string) (string, error) {
	result := s.bash.Execute(command)
	if !result.Success {
		return result.Output, fmt.Errorf("execute: %s", result.Error)
	}
	return result.Output, nil
}

// ExecuteScript 执行脚本
func (s *StaffBashTool) ExecuteScript(script string) (string, error) {
	result := s.bash.ExecuteScript(script)
	if !result.Success {
		return result.Output, fmt.Errorf("script: %s", result.Error)
	}
	return result.Output, nil
}

// RunGoBuild 编译 Go 代码
func (s *StaffBashTool) RunGoBuild() (string, error) {
	result := s.bash.Execute("go build -o app ./...")
	return result.Output, nil
}

// RunGoTest 运行 Go 测试
func (s *StaffBashTool) RunGoTest() (string, error) {
	result := s.bash.Execute("go test -v ./...")
	return result.Output, nil
}

// RunGoFmt 格式化代码
func (s *StaffBashTool) RunGoFmt() (string, error) {
	result := s.bash.Execute("gofmt -w .")
	return result.Output, nil
}

// ListFiles 列出文件
func (s *StaffBashTool) ListFiles() (string, error) {
	result := s.bash.Execute("find . -type f -name '*.go' | head -20")
	return result.Output, nil
}

// GetHistory 获取执行历史
func (s *StaffBashTool) GetHistory() []CommandRecord {
	return s.bash.GetHistory(50)
}

// sanitize 清理项目名
func sanitize(name string) string {
	replacer := []struct{ old, new string }{
		{" ", "-"},
		{"/", "-"},
		{"\\", "-"},
		{":", "-"},
	}
	for _, r := range replacer {
		name = strings.ReplaceAll(name, r.old, r.new)
	}
	return name
}
