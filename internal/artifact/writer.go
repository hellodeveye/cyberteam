package artifact

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Type Artifact 类型
type Type string

const (
	TypeMarkdown Type = "markdown" // Markdown 文档
	TypeCode     Type = "code"     // 源代码
	TypeTest     Type = "test"     // 测试代码
	TypeConfig   Type = "config"   // 配置文件
	TypeData     Type = "data"     // 结构化数据 (JSON/YAML)
)

// Artifact 单个产出物
type Artifact struct {
	Type     Type
	Path     string            // 相对路径，如 "src/main.go"
	Content  string            // 文件内容
	Metadata map[string]any    // 额外元数据
}

// Writer Artifact 写入器
type Writer struct {
	baseDir string
}

// NewWriter 创建写入器
func NewWriter(projectDir string) *Writer {
	return &Writer{baseDir: projectDir}
}

// WriteArtifact 写入单个 Artifact
func (w *Writer) WriteArtifact(stageNum int, stageName string, art Artifact) error {
	// 构建完整路径: workspaces/<project>/04-develop/src/main.go
	stageDir := filepath.Join(w.baseDir, fmt.Sprintf("%02d-%s", stageNum, stageName))
	fullPath := filepath.Join(stageDir, art.Path)

	// 确保目录存在
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	// 写入内容
	content := art.Content
	if art.Type == TypeData && len(content) == 0 && art.Metadata != nil {
		// 如果是数据类型且 Content 为空，序列化 Metadata
		if data, err := json.MarshalIndent(art.Metadata, "", "  "); err == nil {
			content = string(data)
		}
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write %s: %w", fullPath, err)
	}

	return nil
}

// WriteArtifacts 批量写入
func (w *Writer) WriteArtifacts(stageNum int, stageName string, artifacts []Artifact) []error {
	var errors []error
	for _, art := range artifacts {
		if err := w.WriteArtifact(stageNum, stageName, art); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

// Layout 定义每个阶段的目录结构
type Layout struct {
	StageNum  int
	StageName string
	Dirs      []string // 需要创建的子目录
}

// DefaultLayouts 默认布局配置
var DefaultLayouts = map[string]Layout{
	"requirement": {
		StageNum:  1,
		StageName: "requirement",
		Dirs:      []string{},
	},
	"design": {
		StageNum:  2,
		StageName: "design",
		Dirs:      []string{"api"},
	},
	"review": {
		StageNum:  3,
		StageName: "review",
		Dirs:      []string{},
	},
	"develop": {
		StageNum:  4,
		StageName: "develop",
		Dirs:      []string{"src", "src/internal", "tests", "docs"},
	},
	"test": {
		StageNum:  5,
		StageName: "test",
		Dirs:      []string{"e2e", "unit"},
	},
	"deploy": {
		StageNum:  6,
		StageName: "deploy",
		Dirs:      []string{"k8s", "scripts"},
	},
}

// SetupStage 初始化阶段目录
func (w *Writer) SetupStage(layout Layout) error {
	stageDir := filepath.Join(w.baseDir, fmt.Sprintf("%02d-%s", layout.StageNum, layout.StageName))
	for _, dir := range layout.Dirs {
		fullDir := filepath.Join(stageDir, dir)
		if err := os.MkdirAll(fullDir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// Parser LLM 输出解析器
type Parser struct{}

// ParseJSONResponse 解析 JSON 格式的 LLM 响应
func (p *Parser) ParseJSONResponse(content string) (map[string]any, error) {
	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ExtractMarkdownFromJSON 从嵌套 JSON 中提取 Markdown 内容
// 如果值是对象，转换为 Markdown 格式
func (p *Parser) ExtractMarkdownFromJSON(data map[string]any, key string) string {
	val, ok := data[key]
	if !ok {
		return ""
	}

	switch v := val.(type) {
	case string:
		return v
	case map[string]any:
		// 将嵌套对象转换为 Markdown
		return p.mapToMarkdown(v, 1)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// mapToMarkdown 将 map 转换为 Markdown 格式
func (p *Parser) mapToMarkdown(data map[string]any, level int) string {
	var sb strings.Builder
	prefix := strings.Repeat("#", level)

	for key, val := range data {
		switch v := val.(type) {
		case string:
			sb.WriteString(fmt.Sprintf("%s %s\n\n%s\n\n", prefix, key, v))
		case map[string]any:
			sb.WriteString(fmt.Sprintf("%s %s\n\n", prefix, key))
			sb.WriteString(p.mapToMarkdown(v, level+1))
		case []any:
			sb.WriteString(fmt.Sprintf("%s %s\n\n", prefix, key))
			for _, item := range v {
				sb.WriteString(fmt.Sprintf("- %v\n", item))
			}
			sb.WriteString("\n")
		default:
			sb.WriteString(fmt.Sprintf("%s %s\n\n%v\n\n", prefix, key, v))
		}
	}

	return sb.String()
}

// Helper 函数：创建 Artifact

// NewMarkdownArtifact 创建 Markdown 文档 Artifact
func NewMarkdownArtifact(path, content string) Artifact {
	return Artifact{
		Type:    TypeMarkdown,
		Path:    path,
		Content: content,
	}
}

// NewCodeArtifact 创建代码文件 Artifact
func NewCodeArtifact(path, content string, metadata map[string]any) Artifact {
	return Artifact{
		Type:     TypeCode,
		Path:     path,
		Content:  content,
		Metadata: metadata,
	}
}

// NewTestArtifact 创建测试文件 Artifact
func NewTestArtifact(path, content string) Artifact {
	return Artifact{
		Type:    TypeTest,
		Path:    path,
		Content: content,
	}
}

// NewDataArtifact 创建数据文件 Artifact (自动 JSON 序列化)
func NewDataArtifact(path string, data map[string]any) Artifact {
	return Artifact{
		Type:     TypeData,
		Path:     path,
		Metadata: data,
	}
}
