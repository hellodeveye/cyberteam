package main

import (
	"cyberteam/internal/llm"
	"cyberteam/internal/profile"
	"cyberteam/internal/protocol"
	"cyberteam/internal/staffutil"
	"cyberteam/internal/tools"
	"cyberteam/internal/worker"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		apiKey  = flag.String("api-key", os.Getenv("OPENAI_API_KEY"), "OpenAI API Key")
		baseURL = flag.String("base-url", getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"), "OpenAI Base URL")
		model   = flag.String("model", getEnv("OPENAI_MODEL", "gpt-4o"), "LLM Model")
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

	// 创建 LLM 客户端（必须配置 API Key）
	if *apiKey == "" {
		fmt.Fprintf(os.Stderr, "错误: 未设置 OPENAI_API_KEY 环境变量\n")
		fmt.Fprintf(os.Stderr, "请设置 API Key 后重试:\n")
		fmt.Fprintf(os.Stderr, "  export OPENAI_API_KEY=your-api-key\n")
		os.Exit(1)
	}
	llmClient := llm.NewOpenAIClient(*apiKey, *baseURL)

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

	// 设置会议处理器（方案二）
	meetingParticipant := staffutil.NewMeetingParticipant("tester", *name, prof, llmClient, *model)
	worker.SetMeetingHandler(&TesterMeetingHandler{
		Participant: meetingParticipant,
		Name:        *name,
	})

	// 设置私聊处理器
	worker.SetPrivateHandler(&TesterPrivateHandler{
		Participant: meetingParticipant,
		Name:        *name,
	})

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

	// 初始化 bash 工具
	var bashTool *tools.BashTool
	if task.WorkspaceDir != "" {
		stageDir := filepath.Join(task.WorkspaceDir, "05-test")
		bashTool = tools.NewBashTool(stageDir)
	}

	prompt := fmt.Sprintf(`你是资深测试工程师。请根据 PRD 和设计文档编写测试用例和测试代码。

PRD：
%s

设计文档：
%s

工作目录: %s/05-test

你可以使用以下 bash 命令来创建测试代码：
- mkdir -p e2e unit （创建目录）
- echo "内容" > 文件名 （写入文件）
- cat > 文件名 << 'EOF' ... EOF （写入多行测试代码）

请输出以下内容（JSON 格式）：
{
  "commands": [
    "mkdir -p e2e unit",
    "cat > unit/main_test.go << 'EOF'\npackage main\n...\nEOF",
    ...
  ],
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
  "test_code": "测试代码（Go/Python/JS 等）",
  "coverage": "覆盖率百分比"
}

要求：
1. 使用 commands 创建测试目录和测试代码文件
2. 测试代码完整可运行
3. 覆盖所有功能点
4. 包含正向、反向、边界测试`, prd, design, task.WorkspaceDir)

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

	// 执行 bash 命令创建测试代码
	if bashTool != nil {
		if commands, ok := output["commands"].([]interface{}); ok && len(commands) > 0 {
			resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   3. 创建测试代码..."}}
			for _, cmd := range commands {
				if cmdStr, ok := cmd.(string); ok && cmdStr != "" {
					result := bashTool.Execute(cmdStr)
					if result.Success {
						resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{fmt.Sprintf("      $ %s", cmdStr)}}
					} else {
						resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{fmt.Sprintf("      ⚠️ %s: %s", cmdStr, result.Error)}}
					}
				}
			}
		}

		// 如果生成了 test_code，也写入文件
		if testCode, ok := output["test_code"].(string); ok && testCode != "" {
			result := bashTool.WriteFile("unit/test_suite.go", []byte(testCode))
			if result.Success {
				resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"      ✓ 写入 unit/test_suite.go"}}
			}
		}

		// 使用新的输出系统写入测试文档
		resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"📝 正在写入测试文档..."}}

		handler := staffutil.NewOutputHandler("tester", task.WorkspaceDir)
		files, err := handler.ProcessAndWrite(task, 5, "test", resp.Content)
		if err != nil {
			resultChan <- protocol.TaskResult{
				TaskID: task.ID,
				Logs:   []string{fmt.Sprintf("⚠️ 写入文件失败: %v", err)},
			}
		} else {
			for _, f := range files {
				if !strings.Contains(f, "metadata") {
					resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{fmt.Sprintf("  ✓ %s", f)}}
				}
			}
		}
	}

	result := protocol.TaskResult{
		TaskID:   task.ID,
		Success:  true,
		Outputs:  output,
		Logs:     []string{fmt.Sprintf("✅ 生成 %d 条测试用例和测试代码，覆盖率 %s", len(testCases), coverage)},
		Duration: time.Since(start).Milliseconds(),
	}
	resultChan <- result
}

func (s *TesterStaff) executeTest(task protocol.Task, resultChan chan<- protocol.TaskResult, start time.Time) {
	code := getString(task.Inputs, "code", "")
	testCasesData, _ := json.Marshal(task.Inputs["test_cases"])

	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"🔍 正在执行测试..."}}

	// 初始化 bash 工具
	var bashTool *tools.BashTool
	if task.WorkspaceDir != "" {
		stageDir := filepath.Join(task.WorkspaceDir, "05-test")
		bashTool = tools.NewBashTool(stageDir)
	}

	prompt := fmt.Sprintf(`你是测试执行专家。请分析代码并模拟执行测试用例，生成测试报告。

待测代码：
%s

测试用例：
%s

工作目录: %s/05-test

你可以使用以下 bash 命令来操作：
- go test -v ./... （运行 Go 测试）
- cat > report.md << 'EOF' ... EOF （写入测试报告）

请输出以下内容（JSON 格式）：
{
  "commands": [
    "go test -v ./... 2>&1 | tee test.log",
    "echo '# 测试报告' > report.md",
    ...
  ],
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
3. 给出具体的 Bug 描述`, code, string(testCasesData), task.WorkspaceDir)

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

	// 执行 bash 命令（如运行测试）
	if bashTool != nil {
		if commands, ok := output["commands"].([]interface{}); ok && len(commands) > 0 {
			resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   3. 执行测试命令..."}}
			for _, cmd := range commands {
				if cmdStr, ok := cmd.(string); ok && cmdStr != "" {
					result := bashTool.Execute(cmdStr)
					if result.Success {
						resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{fmt.Sprintf("      $ %s", cmdStr)}}
					} else {
						resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{fmt.Sprintf("      ⚠️ %s: %s", cmdStr, result.Error)}}
					}
				}
			}
		}

		// 使用新的输出系统写入测试报告
		resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"📝 正在写入测试报告..."}}

		handler := staffutil.NewOutputHandler("tester", task.WorkspaceDir)
		files, err := handler.ProcessAndWrite(task, 5, "test", resp.Content)
		if err != nil {
			resultChan <- protocol.TaskResult{
				TaskID: task.ID,
				Logs:   []string{fmt.Sprintf("⚠️ 写入文件失败: %v", err)},
			}
		} else {
			for _, f := range files {
				if !strings.Contains(f, "metadata") {
					resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{fmt.Sprintf("  ✓ %s", f)}}
				}
			}
		}
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

// TesterMeetingHandler Tester 会议处理器
type TesterMeetingHandler struct {
	Participant *staffutil.MeetingParticipant
	Name        string
}

// HandleMeetingMessage 处理会议消息
func (h *TesterMeetingHandler) HandleMeetingMessage(meetingID string, from string, content string, mentioned bool, transcript string) string {
	return h.Participant.GenerateReply(meetingID, "", transcript, from, content, mentioned)
}

// TesterPrivateHandler Tester 私聊处理器
type TesterPrivateHandler struct {
	Participant *staffutil.MeetingParticipant
	Name        string
}

// HandlePrivateMessage 处理私聊消息
func (h *TesterPrivateHandler) HandlePrivateMessage(from string, content string) string {
	return h.Participant.GenerateReply("", "私聊", "", from, content, true)
}
