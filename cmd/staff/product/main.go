package main

import (
	"fmt"
	"os"
	"time"

	"cyberteam/internal/llm"
	"cyberteam/internal/profile"
	"cyberteam/internal/protocol"
	"cyberteam/internal/staffutil"
	"cyberteam/internal/worker"
	"encoding/json"
)

// ProductStaff 产品经理
type ProductStaff struct {
	*worker.BaseWorker
	llmClient llm.Client
	model     string
	profile   *profile.Profile
}

func main() {
	cfg := staffutil.ParseFlags("product")
	cfg.LoadProfile(getDefaultProfile())
	cfg.LoadMemory("")

	staff := &ProductStaff{
		llmClient: cfg.LLMClient,
		model:     cfg.Model,
		profile:   cfg.Profile,
	}
	staff.BaseWorker = cfg.SetupWorker("product", staff)

	if err := staff.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Product staff error: %v\n", err)
		os.Exit(1)
	}
}

// Handle 处理任务
func (s *ProductStaff) Handle(task protocol.Task, resultChan chan<- protocol.TaskResult) {
	start := time.Now()

	switch task.Type {
	case "analyze_requirement":
		s.analyzeRequirement(task, resultChan, start)
	case "design_review":
		s.designReview(task, resultChan, start)
	default:
		resultChan <- protocol.TaskResult{
			TaskID:  task.ID,
			Success: false,
			Error:   fmt.Sprintf("unknown task type: %s", task.Type),
			Logs:    []string{"错误：无法处理该任务类型"},
		}
	}
	close(resultChan)
}

func (s *ProductStaff) analyzeRequirement(task protocol.Task, resultChan chan<- protocol.TaskResult, start time.Time) {
	req := staffutil.GetString(task.Inputs, "requirement", "")
	constraints := staffutil.GetString(task.Inputs, "constraints", "")

	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"📝 正在分析需求..."}}

	prompt := fmt.Sprintf(`你是一个资深产品经理。请根据以下需求撰写一份详细的 PRD 文档。

需求描述：%s
约束条件：%s

请输出以下内容（JSON 格式）：
{
  "prd": "PRD 文档内容",
  "user_stories": ["故事 1", "故事 2"],
  "acceptance_criteria": ["标准 1", "标准 2"]
}

要求：
1. PRD 包含背景、目标、功能描述、验收标准
2. 用户故事遵循 "作为...我希望...以便..." 格式
3. 验收标准可测试`, req, constraints)

	systemPrompt := s.profile.BuildSystemPrompt("analyze_requirement")
	resp, err := s.llmClient.Complete([]llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}, &llm.CompleteOptions{
		Model:       s.model,
		Temperature: 0.7,
		MaxTokens:   2000,
	})

	if err != nil {
		resultChan <- protocol.TaskResult{
			TaskID:   task.ID,
			Success:  false,
			Error:    fmt.Sprintf("LLM error: %v", err),
			Logs:     []string{"❌ LLM 调用失败：" + err.Error()},
			Duration: time.Since(start).Milliseconds(),
		}
		return
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(resp.Content), &output); err != nil {
		output = map[string]any{
			"prd":                 resp.Content,
			"user_stories":        []string{"作为用户，我希望使用这个功能"},
			"acceptance_criteria": []string{"功能可用", "界面友好"},
		}
	}

	if task.WorkspaceDir != "" {
		resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"📝 正在写入 PRD 文档..."}}

		handler := staffutil.NewOutputHandler("product", task.WorkspaceDir)
		files, err := handler.ProcessAndWrite(task, 1, "requirement", resp.Content)
		if err != nil {
			resultChan <- protocol.TaskResult{
				TaskID: task.ID,
				Logs:   []string{fmt.Sprintf("⚠️ 写入文件失败: %v", err)},
			}
		} else {
			for _, f := range files {
				resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{fmt.Sprintf("  ✓ %s", f)}}
			}
		}
	}

	result := protocol.TaskResult{
		TaskID:   task.ID,
		Success:  true,
		Outputs:  output,
		Logs:     []string{fmt.Sprintf("✅ PRD 撰写完成，使用 %d tokens", resp.Usage.TotalTokens)},
		Duration: time.Since(start).Milliseconds(),
	}
	resultChan <- result
}

func (s *ProductStaff) designReview(task protocol.Task, resultChan chan<- protocol.TaskResult, start time.Time) {
	design := staffutil.GetString(task.Inputs, "design", "")
	prd := staffutil.GetString(task.Inputs, "prd", "")

	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"🔍 正在评审设计方案..."}}

	prompt := fmt.Sprintf(`你是产品经理，请评审以下设计方案是否符合 PRD 要求。

PRD：
%s

设计方案：
%s

请输出（JSON 格式）：
{
  "approved": true/false,
  "feedback": "评审意见",
  "suggestions": ["建议 1", "建议 2"]
}

评审要点：
1. 是否满足所有功能需求
2. 技术方案是否可行
3. 用户体验是否良好
4. 是否有遗漏或风险`, prd, design)

	systemPrompt := s.profile.BuildSystemPrompt("design_review")
	resp, err := s.llmClient.Complete([]llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}, nil)

	if err != nil {
		resultChan <- protocol.TaskResult{
			TaskID:   task.ID,
			Success:  false,
			Error:    err.Error(),
			Logs:     []string{"❌ 评审失败"},
			Duration: time.Since(start).Milliseconds(),
		}
		return
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(resp.Content), &output); err != nil {
		output = map[string]any{
			"approved":    true,
			"feedback":    resp.Content,
			"suggestions": []string{},
		}
	}

	approved, _ := output["approved"].(bool)
	status := "✅ 评审通过"
	if !approved {
		status = "❌ 评审未通过，需要修改"
	}

	result := protocol.TaskResult{
		TaskID:   task.ID,
		Success:  true,
		Outputs:  output,
		Logs:     []string{status, fmt.Sprintf("反馈：%s", output["feedback"])},
		Duration: time.Since(start).Milliseconds(),
	}
	resultChan <- result
}

func getDefaultProfile() *profile.Profile {
	return &profile.Profile{
		Name:        "张产品",
		Role:        "product",
		Version:     "1.0.0",
		Description: "资深互联网产品经理，负责需求分析和产品设计",
		Body: `# 角色详细说明

## 工作职责
- 分析用户需求，撰写 PRD 文档
- 组织设计评审，确保方案可行
- 跟进项目进度，协调资源

## 工作原则
1. 用户价值优先
2. 需求必须可测试、可验收
3. 评审严格但建设性`,
	}
}
