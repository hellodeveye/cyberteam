package staffutil

import (
	"cyberteam/internal/adapter"
	"cyberteam/internal/artifact"
	"cyberteam/internal/protocol"
	"encoding/json"
	"fmt"
)

// OutputHandler 统一的输出处理器
type OutputHandler struct {
	role       string
	writer     *artifact.Writer
	adapter    adapter.StaffOutputAdapter
}

// NewOutputHandler 创建输出处理器
func NewOutputHandler(role, workspaceDir string) *OutputHandler {
	return &OutputHandler{
		role:    role,
		writer:  artifact.NewWriter(workspaceDir),
		adapter: adapter.Factory(role),
	}
}

// SetupStage 初始化阶段目录结构
func (h *OutputHandler) SetupStage(stageName string) error {
	if layout, ok := artifact.DefaultLayouts[stageName]; ok {
		return h.writer.SetupStage(layout)
	}
	return nil
}

// ProcessAndWrite 处理 LLM 输出并写入文件
// 
// 参数:
//   - task: 当前任务
//   - stageNum: 阶段编号 (1-6)
//   - stageName: 阶段名称
//   - llmContent: LLM 返回的原始内容 (JSON 字符串)
//
// 返回:
//   - 写入的文件列表
//   - 错误信息
func (h *OutputHandler) ProcessAndWrite(
	task protocol.Task,
	stageNum int,
	stageName string,
	llmContent string,
) ([]string, error) {
	// 1. 解析 LLM 输出
	var output map[string]any
	if err := json.Unmarshal([]byte(llmContent), &output); err != nil {
		// 如果不是 JSON，包装成标准格式
		output = map[string]any{
			"content": llmContent,
		}
	}

	// 2. 使用 Adapter 转换为 Artifacts
	artifacts, err := h.adapter.Adapt(task, output)
	if err != nil {
		return nil, fmt.Errorf("adapt output: %w", err)
	}

	// 3. 初始化目录结构
	if err := h.SetupStage(stageName); err != nil {
		return nil, fmt.Errorf("setup stage: %w", err)
	}

	// 4. 写入文件
	var writtenFiles []string
	errors := h.writer.WriteArtifacts(stageNum, stageName, artifacts)
	
	for _, art := range artifacts {
		writtenFiles = append(writtenFiles, art.Path)
	}

	if len(errors) > 0 {
		return writtenFiles, fmt.Errorf("write errors: %v", errors)
	}

	return writtenFiles, nil
}

// SimpleWriteMarkdown 简单写入 Markdown 文件（用于快速迁移）
func (h *OutputHandler) SimpleWriteMarkdown(
	stageNum int,
	stageName string,
	filename string,
	content string,
) error {
	art := artifact.NewMarkdownArtifact(filename, content)
	return h.writer.WriteArtifact(stageNum, stageName, art)
}

// SimpleWriteCode 简单写入代码文件（用于快速迁移）
func (h *OutputHandler) SimpleWriteCode(
	stageNum int,
	stageName string,
	filepath string,
	content string,
) error {
	art := artifact.NewCodeArtifact(filepath, content, nil)
	return h.writer.WriteArtifact(stageNum, stageName, art)
}

// GetWriter 获取底层的 ArtifactWriter
func (h *OutputHandler) GetWriter() *artifact.Writer {
	return h.writer
}
