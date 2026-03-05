// Package workspace 提供工作空间产物管理
package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// WorkspaceManager 定义工作空间管理器接口
type WorkspaceManager interface {
	CreateProjectWorkspace(projectID, projectName string) (string, error)
	GetProjectDir(projectName, projectID string) string
	GetStageDir(projectName, projectID string, stageNum int) string
	WriteFile(projectName, projectID string, stageNum int, filename string, content []byte) error
	ReadFile(projectName, projectID string, stageNum int, filename string) ([]byte, error)
	SaveArtifact(projectName, projectID string, stageNum int, artifact *Artifact) error
	ReadArtifact(projectName, projectID string, stageNum int) (*Artifact, error)
	ReadDocument(projectName, projectID string, stageNum int) (string, error)
	ReadCodeFile(projectName, projectID string, stageNum int, filename string) (string, error)
	ListStageFiles(projectName, projectID string, stageNum int) ([]string, error)
}

// Artifact 表示一个阶段的工作产物
type Artifact struct {
	// Document 是人类可读的主文档（Markdown格式）
	// 例如：PRD.md、design.md、review-report.md
	Document string

	// CodeFiles 是代码文件映射，key为文件名，value为内容
	// 例如：{"main.go": "...", "go.mod": "..."}
	CodeFiles map[string]string

	// Metadata 是机器用的元数据（token消耗、执行时间等）
	Metadata map[string]interface{}
}

// StageArtifactNames 定义各阶段的标准产物文件名
type StageArtifactNames struct {
	Document   string   // 主文档文件名
	CodeFiles  []string // 预期代码文件列表
	MetaFile   string   // 元数据文件名
}

// StageArtifacts 各阶段的标准产物定义
var StageArtifacts = map[int]StageArtifactNames{
	1: { // requirement
		Document:  "PRD.md",
		CodeFiles: []string{},
		MetaFile:  "metadata.json",
	},
	2: { // design
		Document:  "design.md",
		CodeFiles: []string{},
		MetaFile:  "metadata.json",
	},
	3: { // review
		Document:  "review-report.md",
		CodeFiles: []string{},
		MetaFile:  "metadata.json",
	},
	4: { // develop
		Document:  "README.md",
		CodeFiles: []string{"main.go", "go.mod", "main_test.go"},
		MetaFile:  "metadata.json",
	},
	5: { // test
		Document:  "test-report.md",
		CodeFiles: []string{},
		MetaFile:  "metadata.json",
	},
	6: { // deploy
		Document:  "deploy-guide.md",
		CodeFiles: []string{"Dockerfile", "docker-compose.yml"},
		MetaFile:  "metadata.json",
	},
}

// StageNumber 阶段名称到数字的映射
var StageNumber = map[string]int{
	"requirement": 1,
	"design":      2,
	"review":      3,
	"develop":     4,
	"test":        5,
	"deploy":      6,
}

// StageName 数字到阶段名称的映射
var StageName = map[int]string{
	1: "requirement",
	2: "design",
	3: "review",
	4: "develop",
	5: "test",
	6: "deploy",
}

// StageDirName 获取阶段目录名
func StageDirName(stageNum int) string {
	names := map[int]string{
		1: "01-requirement",
		2: "02-design",
		3: "03-review",
		4: "04-develop",
		5: "05-test",
		6: "06-deploy",
	}
	if name, ok := names[stageNum]; ok {
		return name
	}
	return "docs"
}

// SaveArtifact 保存产物到工作空间
// 参数：projectName, projectID, stageNum 用于定位目录
func (m *Manager) SaveArtifact(projectName, projectID string, stageNum int, artifact *Artifact) error {
	stageDir := filepath.Join(
		m.GetProjectDir(projectName, projectID),
		StageDirName(stageNum),
	)

	if err := os.MkdirAll(stageDir, 0755); err != nil {
		return fmt.Errorf("create stage dir: %w", err)
	}

	names := StageArtifacts[stageNum]

	// 1. 保存主文档
	if artifact.Document != "" && names.Document != "" {
		docPath := filepath.Join(stageDir, names.Document)
		if err := os.WriteFile(docPath, []byte(artifact.Document), 0644); err != nil {
			return fmt.Errorf("write document: %w", err)
		}
	}

	// 2. 保存代码文件
	for filename, content := range artifact.CodeFiles {
		if content == "" {
			continue
		}
		codePath := filepath.Join(stageDir, filename)
		if err := os.WriteFile(codePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("write code file %s: %w", filename, err)
		}
	}

	// 3. 保存元数据
	if artifact.Metadata != nil && len(artifact.Metadata) > 0 {
		metaPath := filepath.Join(stageDir, names.MetaFile)
		metaJSON, err := json.MarshalIndent(artifact.Metadata, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
		if err := os.WriteFile(metaPath, metaJSON, 0644); err != nil {
			return fmt.Errorf("write metadata: %w", err)
		}
	}

	return nil
}

// ReadArtifact 从工作空间读取产物
func (m *Manager) ReadArtifact(projectName, projectID string, stageNum int) (*Artifact, error) {
	stageDir := filepath.Join(
		m.GetProjectDir(projectName, projectID),
		StageDirName(stageNum),
	)

	names := StageArtifacts[stageNum]
	artifact := &Artifact{
		CodeFiles: make(map[string]string),
		Metadata:  make(map[string]interface{}),
	}

	// 1. 读取主文档
	if names.Document != "" {
		docPath := filepath.Join(stageDir, names.Document)
		if data, err := os.ReadFile(docPath); err == nil {
			artifact.Document = string(data)
		}
	}

	// 2. 读取代码文件
	for _, filename := range names.CodeFiles {
		codePath := filepath.Join(stageDir, filename)
		if data, err := os.ReadFile(codePath); err == nil {
			artifact.CodeFiles[filename] = string(data)
		}
	}

	// 3. 读取元数据
	if names.MetaFile != "" {
		metaPath := filepath.Join(stageDir, names.MetaFile)
		if data, err := os.ReadFile(metaPath); err == nil {
			json.Unmarshal(data, &artifact.Metadata)
		}
	}

	return artifact, nil
}

// ReadDocument 读取指定阶段的主文档
func (m *Manager) ReadDocument(projectName, projectID string, stageNum int) (string, error) {
	names := StageArtifacts[stageNum]
	if names.Document == "" {
		return "", fmt.Errorf("no document defined for stage %d", stageNum)
	}

	stageDir := filepath.Join(
		m.GetProjectDir(projectName, projectID),
		StageDirName(stageNum),
	)

	docPath := filepath.Join(stageDir, names.Document)
	data, err := os.ReadFile(docPath)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// ReadCodeFile 读取指定阶段的代码文件
func (m *Manager) ReadCodeFile(projectName, projectID string, stageNum int, filename string) (string, error) {
	stageDir := filepath.Join(
		m.GetProjectDir(projectName, projectID),
		StageDirName(stageNum),
	)

	codePath := filepath.Join(stageDir, filename)
	data, err := os.ReadFile(codePath)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// ListStageFiles 列出阶段目录下的所有文件
func (m *Manager) ListStageFiles(projectName, projectID string, stageNum int) ([]string, error) {
	stageDir := filepath.Join(
		m.GetProjectDir(projectName, projectID),
		StageDirName(stageNum),
	)

	entries, err := os.ReadDir(stageDir)
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

// ParseMarkdownCodeBlocks 从 Markdown 中提取代码块
// 返回 map[语言]代码内容
func ParseMarkdownCodeBlocks(markdown string) map[string]string {
	result := make(map[string]string)

	// 匹配 ```lang\ncode\n``` 或 ```\ncode\n```
	// (?s) 开启单行模式，使 . 匹配换行符
	pattern := regexp.MustCompile("(?s)```(\\w*)\\n(.*?)\\n```")
	matches := pattern.FindAllStringSubmatch(markdown, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			lang := match[1]
			code := strings.TrimSpace(match[2])
			if lang == "" {
				lang = "text"
			}
			// 如果有多个同语言代码块，追加
			if existing, ok := result[lang]; ok {
				result[lang] = existing + "\n\n" + code
			} else {
				result[lang] = code
			}
		}
	}

	return result
}

// ExtractDocumentFromMarkdown 从 Markdown 中提取文档部分（去除代码块）
func ExtractDocumentFromMarkdown(markdown string) string {
	// 移除代码块，保留其他内容
	pattern := regexp.MustCompile("(?s)```.*?```")
	doc := pattern.ReplaceAllString(markdown, "")

	// 清理多余空行
	doc = regexp.MustCompile("\n{3,}").ReplaceAllString(doc, "\n\n")
	doc = strings.TrimSpace(doc)

	return doc
}

// TaskResultToArtifact 将 TaskResult.Outputs 转换为 Artifact
// 用于兼容现有 JSON 输出的 Staff
func TaskResultToArtifact(outputs map[string]interface{}, stageNum int) *Artifact {
	artifact := &Artifact{
		CodeFiles: make(map[string]string),
		Metadata:  make(map[string]interface{}),
	}

	names := StageArtifacts[stageNum]

	// 尝试提取文档
	docKeys := []string{"prd", "design", "document", "content", "output", "feedback"}
	for _, key := range docKeys {
		if val, ok := outputs[key]; ok {
			if s, ok := val.(string); ok && s != "" {
				artifact.Document = s
				break
			}
		}
	}

	// 尝试提取代码
	codeKeys := []string{"code", "fixed_code", "implementation"}
	for _, key := range codeKeys {
		if val, ok := outputs[key]; ok {
			if s, ok := val.(string); ok && s != "" {
				// 根据阶段确定文件名
				if stageNum == 4 { // develop
					artifact.CodeFiles["main.go"] = s
				}
				break
			}
		}
	}

	// 如果文档中包含代码块，尝试提取
	if artifact.Document != "" {
		codeBlocks := ParseMarkdownCodeBlocks(artifact.Document)
		for lang, code := range codeBlocks {
			// 根据语言推断文件名
			filename := langToFilename(lang, stageNum)
			if filename != "" && artifact.CodeFiles[filename] == "" {
				artifact.CodeFiles[filename] = code
			}
		}

		// 重新提取纯文档部分（去除代码块）
		doc := ExtractDocumentFromMarkdown(artifact.Document)
		if doc != "" {
			artifact.Document = doc
		}
	}

	// 提取元数据（保留原始 outputs 中未处理的字段）
	for key, val := range outputs {
		isDoc := false
		for _, dk := range docKeys {
			if key == dk {
				isDoc = true
				break
			}
		}
		isCode := false
		for _, ck := range codeKeys {
			if key == ck {
				isCode = true
				break
			}
		}
		if !isDoc && !isCode {
			artifact.Metadata[key] = val
		}
	}

	// 确保有文档文件名
	if artifact.Document != "" && names.Document != "" {
		artifact.Metadata["document_filename"] = names.Document
	}

	return artifact
}

// langToFilename 根据语言推断文件名
func langToFilename(lang string, stageNum int) string {
	// 语言到文件扩展名的映射
	extMap := map[string]string{
		"go":         ".go",
		"python":     ".py",
		"javascript": ".js",
		"typescript": ".ts",
		"java":       ".java",
		"rust":       ".rs",
		"c":          ".c",
		"cpp":        ".cpp",
		"yaml":       ".yaml",
		"yml":        ".yml",
		"json":       ".json",
		"dockerfile": "Dockerfile",
	}

	ext, ok := extMap[strings.ToLower(lang)]
	if !ok {
		return ""
	}

	// 根据阶段和扩展名推断文件名
	if stageNum == 4 { // develop
		if ext == ".go" {
			return "main.go"
		}
		return "main" + ext
	}
	if stageNum == 6 { // deploy
		if ext == ".yml" || ext == ".yaml" {
			return "docker-compose.yml"
		}
		if strings.ToLower(lang) == "dockerfile" {
			return "Dockerfile"
		}
	}

	return ""
}

// LegacyJSONFilename 返回旧版 JSON 产物文件名（用于兼容）
func LegacyJSONFilename(stageNum int) string {
	stage := StageName[stageNum]
	if stage == "" {
		return "output.json"
	}
	return stage + "-output.json"
}
