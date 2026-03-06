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
		apiKey  = flag.String("api-key", os.Getenv("OPENAI_API_KEY"), "OpenAI API Key")
		baseURL = flag.String("base-url", getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"), "OpenAI Base URL")
		model   = flag.String("model", getEnv("OPENAI_MODEL", "gpt-4o"), "LLM Model")
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

	// 设置会议处理器（方案二）
	meetingParticipant := staffutil.NewMeetingParticipant("developer", *name, prof, llmClient, *model)
	worker.SetMeetingHandler(&StaffMeetingHandler{
		Participant: meetingParticipant,
		Name:        *name,
	})

	// 设置私聊处理器
	worker.SetPrivateHandler(&StaffPrivateHandler{
		Participant: meetingParticipant,
		Name:        *name,
	})

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

工作目录: %s/02-design

你可以使用以下 bash 命令来创建设计文档结构：
- mkdir -p api docs （创建目录）
- echo "内容" > 文件名 （写入文件）

请输出以下内容（JSON 格式）：
{
  "commands": [
    "mkdir -p api docs",
    "echo '# API 设计' > api/endpoints.md",
    ...
  ],
  "design": "详细设计文档（包含模块划分、接口定义、数据模型）",
  "architecture": "架构图描述（用文本描述）",
  "tech_stack": ["Go", "PostgreSQL", "Redis"]
}

设计要求：
1. 模块化设计，职责清晰
2. 考虑扩展性和可维护性
3. 明确技术选型理由
4. 包含关键接口定义`, prd, feedback, task.WorkspaceDir)

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

	// 执行 bash 命令创建设计文档结构
	if task.WorkspaceDir != "" {
		stageDir := filepath.Join(task.WorkspaceDir, "02-design")
		bashTool := tools.NewBashTool(stageDir)

		if commands, ok := output["commands"].([]interface{}); ok && len(commands) > 0 {
			resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   6. 创建设计文档结构..."}}
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

		// 使用新的输出系统写入文件（作为备份）
		resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   7. 写入设计文档..."}}

		handler := staffutil.NewOutputHandler("developer", task.WorkspaceDir)
		files, err := handler.ProcessAndWrite(task, 2, "design", resp.Content)
		if err != nil {
			resultChan <- protocol.TaskResult{
				TaskID: task.ID,
				Logs:   []string{fmt.Sprintf("⚠️ 写入文件失败: %v", err)},
			}
		} else {
			for _, f := range files {
				if !strings.Contains(f, "metadata") {
					resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{fmt.Sprintf("      ✓ %s", f)}}
				}
			}
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
	time.Sleep(300 * time.Millisecond)
	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   2. 设计代码结构..."}}
	time.Sleep(300 * time.Millisecond)

	// 初始化 bash 工具
	var bashTool *tools.BashTool
	if task.WorkspaceDir != "" {
		stageDir := filepath.Join(task.WorkspaceDir, "04-develop")
		bashTool = tools.NewBashTool(stageDir)
	}

	prompt := fmt.Sprintf(`你是资深 %s 开发工程师。请根据设计文档和 PRD 实现代码。

PRD：
%s

设计文档：
%s

工作目录: %s

你可以使用以下 bash 命令来创建项目结构和文件：
- mkdir -p 目录名  （创建目录）
- echo "内容" > 文件名  （写入文件）
- cat > 文件名 << 'EOF' ... EOF  （写入多行文件）

请输出以下内容（JSON 格式）：
{
  "commands": [
    "mkdir -p internal/service",
    "echo 'package main' > main.go",
    ...
  ],
  "code": "主程序代码（包含完整实现）",
  "tests": "单元测试代码",
  "docs": "接口文档（Markdown 格式）"
}

要求：
1. 先使用 commands 创建项目结构和目录
2. 代码完整可运行
3. 包含错误处理
4. 遵循最佳实践
5. 添加必要注释`, language, prd, design, task.WorkspaceDir)

	systemPrompt := s.profile.BuildSystemPrompt("implement_feature")
	resp, err := s.llmClient.Complete([]llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}, &llm.CompleteOptions{
		Model:       s.model,
		Temperature: 0.2,
		MaxTokens:   4000,
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

	// 执行 bash 命令创建项目结构
	if bashTool != nil {
		if commands, ok := output["commands"].([]interface{}); ok && len(commands) > 0 {
			resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   3. 创建项目结构..."}}
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
	}

	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   4. 添加错误处理..."}}
	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   5. 编写单元测试..."}}
	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   6. 生成接口文档..."}}

	// 使用新的输出系统写入文件（作为备份）
	if task.WorkspaceDir != "" {
		resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   7. 写入代码到工作空间..."}}

		handler := staffutil.NewOutputHandler("developer", task.WorkspaceDir)
		files, err := handler.ProcessAndWrite(task, 4, "develop", resp.Content)
		if err != nil {
			resultChan <- protocol.TaskResult{
				TaskID: task.ID,
				Logs:   []string{fmt.Sprintf("⚠️ 写入文件失败: %v", err)},
			}
		} else {
			for _, f := range files {
				if !strings.Contains(f, "metadata") {
					resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{fmt.Sprintf("      ✓ %s", f)}}
				}
			}
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

// StaffMeetingHandler Staff 会议处理器
type StaffMeetingHandler struct {
	Participant *staffutil.MeetingParticipant
	Name        string
}

// HandleMeetingMessage 处理会议消息
func (h *StaffMeetingHandler) HandleMeetingMessage(meetingID string, from string, content string, mentioned bool, transcript string) string {
	// 使用传入的会议历史
	return h.Participant.GenerateReply(meetingID, "", transcript, from, content, mentioned)
}

// StaffPrivateHandler Staff 私聊处理器
type StaffPrivateHandler struct {
	Participant *staffutil.MeetingParticipant
	Name        string
}

// HandlePrivateMessage 处理私聊消息
func (h *StaffPrivateHandler) HandlePrivateMessage(from string, content string) string {
	// 私聊就是一对一的会议，mentioned 为 true
	return h.Participant.GenerateReply("", "私聊", "", from, content, true)
}
