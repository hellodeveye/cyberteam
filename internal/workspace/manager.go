package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manager 工作空间管理器
type Manager struct {
	baseDir string
}

// NewManager 创建工作空间管理器
func NewManager(baseDir string) *Manager {
	return &Manager{baseDir: baseDir}
}

// CreateProjectWorkspace 创建项目工作空间
func (m *Manager) CreateProjectWorkspace(projectID, projectName string) (string, error) {
	projectDir := filepath.Join(m.baseDir, sanitize(projectName)+"-"+projectID[:8])

	dirs := []string{
		projectDir,
		filepath.Join(projectDir, "01-requirement"), // 产品经理
		filepath.Join(projectDir, "02-design"),      // 架构师/设计
		filepath.Join(projectDir, "03-review"),      // 评审记录
		filepath.Join(projectDir, "04-develop"),     // 开发代码
		filepath.Join(projectDir, "05-test"),        // 测试报告
		filepath.Join(projectDir, "06-deploy"),      // 部署配置
		filepath.Join(projectDir, "docs"),           // 项目文档
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	// 创建 README.md
	readme := fmt.Sprintf("# %s\n\n项目ID: %s\n\n## 目录结构\n\n"+
		"- `01-requirement/` - 需求文档（产品经理）\n"+
		"- `02-design/` - 设计文档\n"+
		"- `03-review/` - 评审记录\n"+
		"- `04-develop/` - 源代码\n"+
		"- `05-test/` - 测试报告\n"+
		"- `06-deploy/` - 部署配置\n"+
		"- `docs/` - 项目文档\n",
		projectName, projectID)

	readmePath := filepath.Join(projectDir, "README.md")
	if err := os.WriteFile(readmePath, []byte(readme), 0644); err != nil {
		return "", err
	}

	return projectDir, nil
}

// GetProjectDir 获取项目目录
func (m *Manager) GetProjectDir(projectName, projectID string) string {
	return filepath.Join(m.baseDir, sanitize(projectName)+"-"+projectID[:8])
}

// GetStageDir 获取阶段目录
func (m *Manager) GetStageDir(projectName, projectID string, stageNum int) string {
	projectDir := m.GetProjectDir(projectName, projectID)
	stageDirs := map[int]string{
		1: "01-requirement",
		2: "02-design",
		3: "03-review",
		4: "04-develop",
		5: "05-test",
		6: "06-deploy",
	}
	if dir, ok := stageDirs[stageNum]; ok {
		return filepath.Join(projectDir, dir)
	}
	return filepath.Join(projectDir, "docs")
}

// WriteFile 写入文件到工作空间
func (m *Manager) WriteFile(projectName, projectID string, stageNum int, filename string, content []byte) error {
	dir := m.GetStageDir(projectName, projectID, stageNum)
	filepath := filepath.Join(dir, filename)
	return os.WriteFile(filepath, content, 0644)
}

// ReadFile 读取文件
func (m *Manager) ReadFile(projectName, projectID string, stageNum int, filename string) ([]byte, error) {
	dir := m.GetStageDir(projectName, projectID, stageNum)
	filepath := filepath.Join(dir, filename)
	return os.ReadFile(filepath)
}

// ListFiles 列出阶段目录下的文件
func (m *Manager) ListFiles(projectName, projectID string, stageNum int) ([]string, error) {
	dir := m.GetStageDir(projectName, projectID, stageNum)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}

// ListAllArtifacts 列出所有产出物
func (m *Manager) ListAllArtifacts(projectName, projectID string) map[string][]string {
	result := make(map[string][]string)
	projectDir := m.GetProjectDir(projectName, projectID)

	stages := []string{
		"01-requirement",
		"02-design",
		"03-review",
		"04-develop",
		"05-test",
		"06-deploy",
		"docs",
	}

	for _, stage := range stages {
		dir := filepath.Join(projectDir, stage)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		var files []string
		for _, entry := range entries {
			if !entry.IsDir() {
				files = append(files, entry.Name())
			}
		}
		if len(files) > 0 {
			result[stage] = files
		}
	}

	return result
}

// sanitize 清理目录名
func sanitize(name string) string {
	// 替换不安全字符
	replacer := []struct {
		old string
		new string
	}{
		{" ", "-"},
		{"/", "-"},
		{"\\", "-"},
		{":", "-"},
		{"*", "-"},
		{"?", "-"},
		{"\"", "-"},
		{"<", "-"},
		{">", "-"},
		{"|", "-"},
	}

	result := name
	for _, r := range replacer {
		result = strings.ReplaceAll(result, r.old, r.new)
	}
	return result
}
