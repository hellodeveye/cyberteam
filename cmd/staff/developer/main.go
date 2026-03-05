package main

import (
	"agent-cluster/internal/llm"
	"agent-cluster/internal/profile"
	"agent-cluster/internal/protocol"
	"agent-cluster/internal/worker"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DeveloperStaff 开发工程师
type DeveloperStaff struct {
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
		fmt.Fprintln(os.Stderr, "Usage: developer --id <id> --name <name>")
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
				"```go\npackage main\n\nfunc TodoApp() {\n    // TODO: implement\n}\n```",
				"好的，我修复了空指针问题，添加了边界检查。",
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
		Role:            "developer",
		Version:         "1.0.0",
		Capabilities:    buildCapabilities(prof),
		Status:          protocol.StatusIdle,
		Load:            0,
		ProfileMarkdown: prof.Body,
	}

	staff := &DeveloperStaff{
		llmClient: llmClient,
		model:     *model,
		profile:   prof,
	}
	staff.BaseWorker = worker.NewBaseWorker(profileData, staff)

	if err := staff.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Developer staff error: %v\n", err)
		os.Exit(1)
	}
}

// Handle 处理任务
func (s *DeveloperStaff) Handle(task protocol.Task, resultChan chan<- protocol.TaskResult) {
	start := time.Now()

	switch task.Type {
	case "design_system":
		s.designSystem(task, resultChan, start)
	case "implement_feature":
		s.implementFeature(task, resultChan, start)
	case "fix_bug":
		s.fixBug(task, resultChan, start)
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

func (s *DeveloperStaff) designSystem(task protocol.Task, resultChan chan<- protocol.TaskResult, start time.Time) {
	prd := getString(task.Inputs, "prd", "")
	feedback := getString(task.Inputs, "feedback", "")

	// 发送详细进度日志
	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"📐 正在进行系统设计..."}}
	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   1. 分析 PRD 需求..."}}
	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   2. 确定技术栈..."}}
	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   3. 设计系统架构..."}}
	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   4. 设计数据模型..."}}
	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   5. 定义接口规范..."}}

	prompt := fmt.Sprintf(`你是资深系统架构师。请根据 PRD 进行系统设计。

PRD：
%s

反馈建议：
%s

请输出以下内容（JSON 格式）：
{
  "design": "详细设计文档（包含模块划分、接口定义、数据模型）",
  "architecture": "架构图描述（用文本描述）",
  "tech_stack": ["Go", "PostgreSQL", "Redis"]
}

设计要求：
1. 模块化设计，职责清晰
2. 考虑扩展性和可维护性
3. 明确技术选型理由
4. 包含关键接口定义`, prd, feedback)

	systemPrompt := s.profile.BuildSystemPrompt("design_system")
	resp, err := s.llmClient.Complete([]llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}, &llm.CompleteOptions{
		Model:       s.model,
		Temperature: 0.3,
		MaxTokens:   3000,
	})

	if err != nil {
		resultChan <- protocol.TaskResult{
			TaskID:   task.ID,
			Success:  false,
			Error:    err.Error(),
			Logs:     []string{"❌ 系统设计失败：" + err.Error()},
			Duration: time.Since(start).Milliseconds(),
		}
		return
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(resp.Content), &output); err != nil {
		output = map[string]any{
			"design":       resp.Content,
			"architecture": "未提供架构图",
			"tech_stack":   []string{"Go"},
		}
	}

	// 将设计文档写入工作目录
	if task.WorkspaceDir != "" {
		resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   6. 写入设计文档到工作空间..."}}

		stageDir := filepath.Join(task.WorkspaceDir, "02-design")
		os.MkdirAll(stageDir, 0755)

		// 写入设计文档
		if design, ok := output["design"].(string); ok && design != "" {
			designPath := filepath.Join(stageDir, "design.md")
			if err := os.WriteFile(designPath, []byte(design), 0644); err == nil {
				resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{fmt.Sprintf("      ✓ 写入 %s", designPath)}}
			}
		}

		// 写入架构图描述
		if arch, ok := output["architecture"].(string); ok && arch != "" {
			archPath := filepath.Join(stageDir, "architecture.md")
			os.WriteFile(archPath, []byte(arch), 0644)
		}

		// 同时写入一个 JSON 格式的完整输出
		outputPath := filepath.Join(stageDir, "design-output.json")
		if outputJSON, err := json.MarshalIndent(output, "", "  "); err == nil {
			os.WriteFile(outputPath, outputJSON, 0644)
		}
	}

	result := protocol.TaskResult{
		TaskID:   task.ID,
		Success:  true,
		Outputs:  output,
		Logs:     []string{fmt.Sprintf("✅ 系统设计完成，使用 %d tokens", resp.Usage.TotalTokens)},
		Duration: time.Since(start).Milliseconds(),
	}
	resultChan <- result
}

func (s *DeveloperStaff) implementFeature(task protocol.Task, resultChan chan<- protocol.TaskResult, start time.Time) {
	design := getString(task.Inputs, "design", "")
	prd := getString(task.Inputs, "prd", "")
	language := getString(task.Inputs, "language", "Go")

	// 发送详细进度日志
	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"💻 开始功能开发..."}}
	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   1. 分析设计文档..."}}
	time.Sleep(500 * time.Millisecond)
	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   2. 设计代码结构..."}}
	time.Sleep(500 * time.Millisecond)
	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   3. 编写核心业务逻辑..."}}
	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   4. 添加错误处理..."}}
	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   5. 编写单元测试..."}}
	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   6. 生成接口文档..."}}

	prompt := fmt.Sprintf(`你是资深 %s 开发工程师。请根据设计文档和 PRD 实现代码。

PRD：
%s

设计文档：
%s

请输出以下内容（JSON 格式）：
{
  "code": "主程序代码（包含完整实现）",
  "tests": "单元测试代码",
  "docs": "接口文档（Markdown 格式）"
}

要求：
1. 代码完整可运行
2. 包含错误处理
3. 遵循最佳实践
4. 添加必要注释`, language, prd, design)

	systemPrompt := s.profile.BuildSystemPrompt("implement_feature")
	resp, err := s.llmClient.Complete([]llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}, &llm.CompleteOptions{
		Model:       s.model,
		Temperature: 0.2,
		MaxTokens:   3000,
	})

	if err != nil {
		resultChan <- protocol.TaskResult{
			TaskID:   task.ID,
			Success:  false,
			Error:    err.Error(),
			Logs:     []string{"❌ 代码生成失败：" + err.Error()},
			Duration: time.Since(start).Milliseconds(),
		}
		return
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(resp.Content), &output); err != nil {
		// 如果不是 JSON，全部放入 code
		output = map[string]any{
			"code":  resp.Content,
			"tests": "// TODO: 添加测试",
			"docs":  "// TODO: 添加文档",
		}
	}

	// 将代码写入工作目录
	if task.WorkspaceDir != "" {
		resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   7. 写入代码到工作空间..."}}

		stageDir := filepath.Join(task.WorkspaceDir, "04-develop")
		os.MkdirAll(stageDir, 0755)

		// 写入主代码
		if code, ok := output["code"].(string); ok && code != "" {
			codePath := filepath.Join(stageDir, "main.go")
			if err := os.WriteFile(codePath, []byte(code), 0644); err == nil {
				resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{fmt.Sprintf("      ✓ 写入 %s", codePath)}}
			}
		}

		// 写入测试代码
		if tests, ok := output["tests"].(string); ok && tests != "" {
			testPath := filepath.Join(stageDir, "main_test.go")
			if err := os.WriteFile(testPath, []byte(tests), 0644); err == nil {
				resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{fmt.Sprintf("      ✓ 写入 %s", testPath)}}
			}
		}

		// 写入文档
		if docs, ok := output["docs"].(string); ok && docs != "" {
			docPath := filepath.Join(stageDir, "README.md")
			if err := os.WriteFile(docPath, []byte(docs), 0644); err == nil {
				resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{fmt.Sprintf("      ✓ 写入 %s", docPath)}}
			}
		}

		// 同时写入一个 JSON 格式的完整输出
		outputPath := filepath.Join(stageDir, "develop-output.json")
		if outputJSON, err := json.MarshalIndent(output, "", "  "); err == nil {
			os.WriteFile(outputPath, outputJSON, 0644)
		}
	}

	result := protocol.TaskResult{
		TaskID:   task.ID,
		Success:  true,
		Outputs:  output,
		Logs:     []string{fmt.Sprintf("✅ 代码开发完成，使用 %d tokens", resp.Usage.TotalTokens)},
		Duration: time.Since(start).Milliseconds(),
	}
	resultChan <- result
}

func (s *DeveloperStaff) fixBug(task protocol.Task, resultChan chan<- protocol.TaskResult, start time.Time) {
	bugsData, _ := json.Marshal(task.Inputs["bugs"])
	code := getString(task.Inputs, "code", "")

	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"🔍 分析 Bug 报告..."}}

	prompt := fmt.Sprintf(`你是资深开发工程师，请修复以下 Bug。

当前代码：
%s

Bug 列表：
%s

请输出以下内容（JSON 格式）：
{
  "fixed_code": "修复后的完整代码",
  "changes": ["修改 1 说明", "修改 2 说明"]
}

修复要求：
1. 完整修复所有 Bug
2. 不引入新问题
3. 添加必要注释说明修改原因`, code, string(bugsData))

	systemPrompt := s.profile.BuildSystemPrompt("fix_bug")
	resp, err := s.llmClient.Complete([]llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}, nil)

	if err != nil {
		resultChan <- protocol.TaskResult{
			TaskID:   task.ID,
			Success:  false,
			Error:    err.Error(),
			Logs:     []string{"❌ Bug 修复失败"},
			Duration: time.Since(start).Milliseconds(),
		}
		return
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(resp.Content), &output); err != nil {
		output = map[string]any{
			"fixed_code": resp.Content,
			"changes":    []string{"根据 LLM 建议修复"},
		}
	}

	changes, _ := output["changes"].([]interface{})
	result := protocol.TaskResult{
		TaskID:   task.ID,
		Success:  true,
		Outputs:  output,
		Logs:     []string{fmt.Sprintf("✅ 修复完成，共 %d 处修改", len(changes))},
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
		Name:        "李开发",
		Role:        "developer",
		Version:     "1.0.0",
		Description: "资深后端开发工程师，擅长 Go 语言和分布式系统",
		Body: `# 角色详细说明

## 工作职责
- 根据设计文档实现功能代码
- 编写单元测试和集成测试
- 修复 Bug，优化性能

## 编码规范
1. 代码完整可运行
2. 包含错误处理
3. 遵循 Go 最佳实践`,
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
			Name:        "design_system",
			Description: "系统设计，输出架构设计文档",
			Inputs: []protocol.Param{
				{Name: "prd", Type: "string", Required: true, Desc: "PRD 文档"},
				{Name: "feedback", Type: "string", Required: false, Desc: "反馈建议"},
			},
			Outputs: []protocol.Param{
				{Name: "design", Type: "string", Desc: "设计文档"},
				{Name: "architecture", Type: "string", Desc: "架构图描述"},
				{Name: "tech_stack", Type: "array", Desc: "技术栈"},
			},
			EstTime: "1h",
		},
		{
			Name:        "implement_feature",
			Description: "根据设计文档实现功能代码",
			Inputs: []protocol.Param{
				{Name: "design", Type: "string", Required: true, Desc: "设计文档"},
				{Name: "prd", Type: "string", Required: true, Desc: "PRD 文档"},
				{Name: "language", Type: "string", Required: false, Desc: "编程语言"},
			},
			Outputs: []protocol.Param{
				{Name: "code", Type: "string", Desc: "实现代码"},
				{Name: "tests", Type: "string", Desc: "测试代码"},
				{Name: "docs", Type: "string", Desc: "接口文档"},
			},
			EstTime: "2h",
		},
		{
			Name:        "fix_bug",
			Description: "修复 Bug",
			Inputs: []protocol.Param{
				{Name: "bugs", Type: "array", Required: true, Desc: "Bug 列表"},
				{Name: "code", Type: "string", Required: true, Desc: "当前代码"},
			},
			Outputs: []protocol.Param{
				{Name: "fixed_code", Type: "string", Desc: "修复后的代码"},
				{Name: "changes", Type: "array", Desc: "修改说明"},
			},
			EstTime: "1h",
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
