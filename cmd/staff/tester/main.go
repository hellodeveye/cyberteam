package main

import (
	"cyber-company/internal/llm"
	"cyber-company/internal/profile"
	"cyber-company/internal/protocol"
	"cyber-company/internal/worker"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TesterStaff 测试工程师
type TesterStaff struct {
	*worker.BaseWorker
	llmClient llm.Client
	model     string
	profile   *profile.Profile
}

func main() {
	var (
		id      = flag.String("id", "", "Staff ID")
		name    = flag.String("name", "", "Staff name")
		apiKey  = flag.String("api-key", os.Getenv("DEEPSEEK_API_KEY"), "DeepSeek API Key")
		baseURL = flag.String("base-url", getEnv("DEEPSEEK_BASE_URL", "https://api.deepseek.com/v1"), "DeepSeek Base URL")
		model   = flag.String("model", getEnv("DEEPSEEK_MODEL", "deepseek-chat"), "LLM Model")
	)
	flag.Parse()

	if *id == "" || *name == "" {
		fmt.Fprintln(os.Stderr, "Usage: tester --id <id> --name <name>")
		os.Exit(1)
	}

	// 获取执行文件所在目录
	execPath, _ := os.Executable()
	execDir := filepath.Dir(execPath)
	profilePath := filepath.Join(execDir, "PROFILE.md")

	var llmClient llm.Client
	if *apiKey != "" {
		llmClient = llm.NewOpenAIClient(*apiKey, *baseURL)
		fmt.Fprintf(os.Stderr, "[%s] 已连接到 DeepSeek: %s\n", *name, *model)
	} else {
		llmClient = &llm.MockClient{
			Responses: []string{
				`{"test_cases": [{"id": "TC-001", "title": "登录成功", "priority": "P0"}], "coverage": "90%"}`,
				`{"report": {"total": 10, "passed": 9, "failed": 1}, "bugs": [{"id": "BUG-001", "severity": "high", "desc": "边界值错误"}]}`,
			},
		}
		fmt.Fprintf(os.Stderr, "[%s] 使用模拟模式（设置 DEEPSEEK_API_KEY 启用真实 LLM）\n", *name)
	}

	// 加载 Profile
	var prof *profile.Profile
	if p, err := profile.Load(profilePath); err == nil {
		prof = p
		fmt.Fprintf(os.Stderr, "[%s] 已加载 PROFILE.md\n", *name)
	} else {
		// Fallback: 使用默认值
		prof = getDefaultProfile()
		fmt.Fprintf(os.Stderr, "[%s] 使用默认 Profile (%v)\n", *name, err)
	}

	profileData := &protocol.WorkerProfile{
		ID:              *id,
		Name:            *name,
		Role:            "tester",
		Version:         "1.0.0",
		Capabilities:    buildCapabilities(prof),
		Status:          protocol.StatusIdle,
		Load:            0,
		ProfileMarkdown: prof.Body,
	}

	staff := &TesterStaff{
		llmClient: llmClient,
		model:     *model,
		profile:   prof,
	}
	staff.BaseWorker = worker.NewBaseWorker(profileData, staff)

	if err := staff.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Tester staff error: %v\n", err)
		os.Exit(1)
	}
}

// Handle 处理任务
func (s *TesterStaff) Handle(task protocol.Task, resultChan chan<- protocol.TaskResult) {
	start := time.Now()

	switch task.Type {
	case "write_test_plan":
		s.writeTestPlan(task, resultChan, start)
	case "execute_test":
		s.executeTest(task, resultChan, start)
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

func (s *TesterStaff) writeTestPlan(task protocol.Task, resultChan chan<- protocol.TaskResult, start time.Time) {
	prd := getString(task.Inputs, "prd", "")
	design := getString(task.Inputs, "design", "")

	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"📋 分析需求，设计测试场景..."}}

	prompt := fmt.Sprintf(`你是资深测试工程师。请根据 PRD 和设计文档编写测试用例。

PRD：
%s

设计文档：
%s

请输出以下内容（JSON 格式）：
{
  "test_cases": [
    {
      "id": "TC-001",
      "title": "用例标题",
      "priority": "P0/P1/P2",
      "type": "positive/negative/boundary",
      "steps": ["步骤 1", "步骤 2"],
      "expected": "预期结果"
    }
  ],
  "coverage": "覆盖率百分比"
}

要求：
1. 覆盖所有功能点
2. 包含正向、反向、边界测试
3. 优先级合理
4. 步骤清晰可执行`, prd, design)

	systemPrompt := s.profile.BuildSystemPrompt("write_test_plan")
	resp, err := s.llmClient.Complete([]llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}, &llm.CompleteOptions{
		Model:       s.model,
		Temperature: 0.3,
		MaxTokens:   2000,
	})

	if err != nil {
		resultChan <- protocol.TaskResult{
			TaskID:   task.ID,
			Success:  false,
			Error:    err.Error(),
			Logs:     []string{"❌ 测试用例生成失败"},
			Duration: time.Since(start).Milliseconds(),
		}
		return
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(resp.Content), &output); err != nil {
		output = map[string]any{
			"test_cases": []interface{}{},
			"coverage":   "N/A",
		}
	}

	testCases, _ := output["test_cases"].([]interface{})
	coverage, _ := output["coverage"].(string)

	result := protocol.TaskResult{
		TaskID:   task.ID,
		Success:  true,
		Outputs:  output,
		Logs:     []string{fmt.Sprintf("✅ 生成 %d 条测试用例，覆盖率 %s", len(testCases), coverage)},
		Duration: time.Since(start).Milliseconds(),
	}
	resultChan <- result
}

func (s *TesterStaff) executeTest(task protocol.Task, resultChan chan<- protocol.TaskResult, start time.Time) {
	code := getString(task.Inputs, "code", "")
	testCasesData, _ := json.Marshal(task.Inputs["test_cases"])

	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"🔍 正在执行测试..."}}

	prompt := fmt.Sprintf(`你是测试执行专家。请分析代码并模拟执行测试用例。

待测代码：
%s

测试用例：
%s

请输出以下内容（JSON 格式）：
{
  "report": {
    "total": 总用例数，
    "passed": 通过数，
    "failed": 失败数，
    "skipped": 跳过数
  },
  "bugs": [
    {
      "id": "BUG-001",
      "severity": "high/medium/low",
      "desc": "Bug 描述",
      "related_case": "TC-001"
    }
  ],
  "passed": true/false
}

注意：
1. 基于代码质量给出真实的测试结果
2. 发现潜在问题
3. 给出具体的 Bug 描述`, code, string(testCasesData))

	systemPrompt := s.profile.BuildSystemPrompt("execute_test")
	resp, err := s.llmClient.Complete([]llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}, nil)

	if err != nil {
		resultChan <- protocol.TaskResult{
			TaskID:   task.ID,
			Success:  false,
			Error:    err.Error(),
			Logs:     []string{"❌ 测试执行失败"},
			Duration: time.Since(start).Milliseconds(),
		}
		return
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(resp.Content), &output); err != nil {
		output = map[string]any{
			"report": map[string]int{"total": 0, "passed": 0, "failed": 0},
			"bugs":   []interface{}{},
			"passed": false,
		}
	}

	report, _ := output["report"].(map[string]interface{})
	bugs, _ := output["bugs"].([]interface{})
	passed, _ := output["passed"].(bool)

	status := "✅ 测试通过"
	if !passed {
		status = fmt.Sprintf("❌ 测试未通过，发现 %d 个 Bug", len(bugs))
	}

	result := protocol.TaskResult{
		TaskID:   task.ID,
		Success:  true,
		Outputs:  output,
		Logs:     []string{status, fmt.Sprintf("报告：%+v", report)},
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
		Name:        "王测试",
		Role:        "tester",
		Version:     "1.0.0",
		Description: "资深测试工程师，负责质量保证",
		Body: `# 角色详细说明

## 工作职责
- 编写测试计划和测试用例
- 执行测试并生成报告
- 跟踪 Bug 修复

## 测试原则
1. 覆盖所有功能点
2. 包含正向、反向、边界测试
3. Bug 描述清晰可复现`,
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

	// 默认 capabilities
	return []protocol.Capability{
		{
			Name:        "write_test_plan",
			Description: "编写测试计划和测试用例",
			Inputs: []protocol.Param{
				{Name: "prd", Type: "string", Required: true, Desc: "PRD 文档"},
				{Name: "design", Type: "string", Required: true, Desc: "设计文档"},
			},
			Outputs: []protocol.Param{
				{Name: "test_cases", Type: "array", Desc: "测试用例列表"},
				{Name: "coverage", Type: "string", Desc: "覆盖率评估"},
			},
			EstTime: "1h",
		},
		{
			Name:        "execute_test",
			Description: "执行测试并生成报告",
			Inputs: []protocol.Param{
				{Name: "code", Type: "string", Required: true, Desc: "待测代码"},
				{Name: "test_cases", Type: "array", Required: true, Desc: "测试用例"},
			},
			Outputs: []protocol.Param{
				{Name: "report", Type: "object", Desc: "测试报告"},
				{Name: "bugs", Type: "array", Desc: "发现的 Bug"},
				{Name: "passed", Type: "bool", Desc: "是否通过"},
			},
			EstTime: "2h",
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
