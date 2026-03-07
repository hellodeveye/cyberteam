package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cyberteam/internal/llm"
	"cyberteam/internal/profile"
	"cyberteam/internal/protocol"
	"cyberteam/internal/staffutil"
	"cyberteam/internal/tools"
	"cyberteam/internal/worker"
)

// DeveloperStaff 开发工程师
type DeveloperStaff struct {
	*worker.BaseWorker
	llmClient llm.Client
	model     string
	profile   *profile.Profile
}

func main() {
	cfg := staffutil.ParseFlags("developer")
	cfg.LoadProfile(getDefaultProfile())
	cfg.LoadMemory("") // 加载 MEMORY.md (可选共享路径留空)

	staff := &DeveloperStaff{
		llmClient: cfg.LLMClient,
		model:     cfg.Model,
		profile:   cfg.Profile,
	}
	staff.BaseWorker = cfg.SetupWorker("developer", staff)

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
	prd := staffutil.GetString(task.Inputs, "prd", "")
	feedback := staffutil.GetString(task.Inputs, "feedback", "")

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
	design := staffutil.GetString(task.Inputs, "design", "")
	prd := staffutil.GetString(task.Inputs, "prd", "")
	language := staffutil.GetString(task.Inputs, "language", "Go")

	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"💻 开始功能开发..."}}
	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   1. 分析设计文档..."}}
	time.Sleep(300 * time.Millisecond)
	resultChan <- protocol.TaskResult{TaskID: task.ID, Logs: []string{"   2. 设计代码结构..."}}
	time.Sleep(300 * time.Millisecond)

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
		output = map[string]any{
			"code":  resp.Content,
			"tests": "// TODO: 添加测试",
			"docs":  "// TODO: 添加文档",
		}
	}

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
	code := staffutil.GetString(task.Inputs, "code", "")

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
