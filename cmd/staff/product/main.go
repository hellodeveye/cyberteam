package main

import (
	"cyberteam/internal/llm"
	"cyberteam/internal/profile"
	"cyberteam/internal/protocol"
	"cyberteam/internal/staffutil"
	"cyberteam/internal/worker"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ProductStaff 产品经理
type ProductStaff struct {
	*worker.BaseWorker
	llmClient llm.Client
	model     string
	profile   *profile.Profile
}

func main() {
	var (
		id      = flag.String("id", "", "Staff ID")
		name    = flag.String("name", "", "Staff name")
		apiKey  = flag.String("api-key", os.Getenv("OPENAI_API_KEY"), "OpenAI API Key")
		baseURL = flag.String("base-url", getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"), "OpenAI Base URL")
		model   = flag.String("model", getEnv("OPENAI_MODEL", "gpt-4o"), "LLM Model")
	)
	flag.Parse()

	if *id == "" || *name == "" {
		fmt.Fprintln(os.Stderr, "Usage: product --id <id> --name <name>")
		os.Exit(1)
	}

	// 获取执行文件所在目录
	execPath, _ := os.Executable()
	execDir := filepath.Dir(execPath)
	profilePath := filepath.Join(execDir, "PROFILE.md")

	// 创建 LLM 客户端
	var llmClient llm.Client
	if *apiKey != "" {
		llmClient = llm.NewOpenAIClient(*apiKey, *baseURL)
	} else {
		llmClient = &llm.MockClient{
			Responses: []string{
				"好的，这是一个优秀的功能需求。我建议采用模块化设计，用户界面要简洁直观。",
				"经过分析，这个功能的核心价值在于提升用户体验，建议优先级定为 P1。",
				"从用户角度看，这个功能能解决痛点，建议尽快落地。",
				"需求很清晰，我这边没有疑问，可以进入设计阶段。",
			},
		}
	}

	// 加载 Profile
	var prof *profile.Profile
	if p, err := profile.Load(profilePath); err == nil {
		prof = p
	} else {
		// Fallback: 使用默认值
		prof = getDefaultProfile()
	}

	profileData := &protocol.WorkerProfile{
		ID:              *id,
		Name:            *name,
		Role:            "product",
		Version:         "1.0.0",
		Capabilities:    buildCapabilities(prof),
		Status:          protocol.StatusIdle,
		Load:            0,
		ProfileMarkdown: prof.Body,
	}

	staff := &ProductStaff{
		llmClient: llmClient,
		model:     *model,
		profile:   prof,
	}
	staff.BaseWorker = worker.NewBaseWorker(profileData, staff)

	// 设置会议处理器（方案二）
	meetingParticipant := staffutil.NewMeetingParticipant("product", *name, prof, llmClient, *model)
	worker.SetMeetingHandler(&ProductMeetingHandler{
		Participant: meetingParticipant,
		Name:        *name,
	})

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
	req := getString(task.Inputs, "requirement", "")
	constraints := getString(task.Inputs, "constraints", "")

	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"📝 正在分析需求..."}}

	// 使用 LLM 生成 PRD
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

	// 尝试解析 JSON
	var output map[string]any
	if err := json.Unmarshal([]byte(resp.Content), &output); err != nil {
		// 如果不是 JSON，包装成 PRD
		output = map[string]any{
			"prd":                 resp.Content,
			"user_stories":        []string{"作为用户，我希望使用这个功能"},
			"acceptance_criteria": []string{"功能可用", "界面友好"},
		}
	}

	// 使用新的输出系统写入文件
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
	design := getString(task.Inputs, "design", "")
	prd := getString(task.Inputs, "prd", "")

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

func getString(m map[string]any, key, defaultVal string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return defaultVal
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// getDefaultProfile 返回默认的 Profile（Fallback 用）
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

// buildCapabilities 从 Profile 构建能力列表
func buildCapabilities(prof *profile.Profile) []protocol.Capability {
	// 如果有 Profile 中定义了 capabilities，使用它们
	if len(prof.Capabilities) > 0 {
		caps := make([]protocol.Capability, len(prof.Capabilities))
		for i, cap := range prof.Capabilities {
			caps[i] = protocol.Capability{
				Name:        cap.Name,
				Description: cap.Description,
				Inputs:      convertParams(cap.Inputs),
				Outputs:     convertParams(cap.Outputs),
				EstTime:     cap.EstTime,
			}
		}
		return caps
	}

	// 默认 capabilities（fallback）
	return []protocol.Capability{
		{
			Name:        "analyze_requirement",
			Description: "分析产品需求，输出 PRD 文档",
			Inputs: []protocol.Param{
				{Name: "requirement", Type: "string", Required: true, Desc: "原始需求描述"},
				{Name: "constraints", Type: "string", Required: false, Desc: "约束条件"},
			},
			Outputs: []protocol.Param{
				{Name: "prd", Type: "string", Desc: "PRD 文档"},
				{Name: "user_stories", Type: "array", Desc: "用户故事"},
				{Name: "acceptance_criteria", Type: "array", Desc: "验收标准"},
			},
			EstTime: "15m",
		},
		{
			Name:        "design_review",
			Description: "评审设计方案",
			Inputs: []protocol.Param{
				{Name: "design", Type: "string", Required: true, Desc: "设计文档"},
				{Name: "prd", Type: "string", Required: true, Desc: "PRD 文档"},
			},
			Outputs: []protocol.Param{
				{Name: "approved", Type: "bool", Desc: "是否通过"},
				{Name: "feedback", Type: "string", Desc: "评审意见"},
				{Name: "suggestions", Type: "array", Desc: "改进建议"},
			},
			EstTime: "10m",
		},
	}
}

// convertParams 转换参数类型
func convertParams(params []profile.Param) []protocol.Param {
	if len(params) == 0 {
		return nil
	}
	result := make([]protocol.Param, len(params))
	for i, p := range params {
		result[i] = protocol.Param{
			Name:     p.Name,
			Type:     p.Type,
			Required: p.Required,
			Desc:     p.Desc,
		}
	}
	return result
}

// ProductMeetingHandler Product 会议处理器
type ProductMeetingHandler struct {
	Participant *staffutil.MeetingParticipant
	Name        string
}

// HandleMeetingMessage 处理会议消息
func (h *ProductMeetingHandler) HandleMeetingMessage(meetingID string, from string, content string, mentioned bool, transcript string) string {
	return h.Participant.GenerateReply(meetingID, "", transcript, from, content, mentioned)
}
