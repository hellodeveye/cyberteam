package adapter

import (
	"cyberteam/internal/artifact"
	"cyberteam/internal/protocol"
	"fmt"
	"strings"
)

// StaffOutputAdapter Staff 输出适配器
type StaffOutputAdapter interface {
	// Adapt 将 LLM 输出转换为 Artifact 列表
	Adapt(task protocol.Task, llmOutput map[string]any) ([]artifact.Artifact, error)
}

// ProductAdapter 产品经理输出适配器
type ProductAdapter struct{}

func (a *ProductAdapter) Adapt(task protocol.Task, output map[string]any) ([]artifact.Artifact, error) {
	var artifacts []artifact.Artifact

	// 1. PRD 文档 - 转换为 Markdown
	prdContent := formatPRDDocument(output)
	artifacts = append(artifacts, artifact.NewMarkdownArtifact(
		"PRD.md",
		prdContent,
	))

	// 2. 用户故事
	if stories, ok := output["user_stories"]; ok {
		content := formatUserStories(stories)
		artifacts = append(artifacts, artifact.NewMarkdownArtifact(
			"user-stories.md",
			content,
		))
	}

	// 3. 元数据备份 (JSON)
	artifacts = append(artifacts, artifact.NewDataArtifact(
		"metadata.json",
		output,
	))

	return artifacts, nil
}

func formatUserStories(stories interface{}) string {
	var sb strings.Builder
	sb.WriteString("# 用户故事\n\n")

	if arr, ok := stories.([]interface{}); ok {
		for i, s := range arr {
			sb.WriteString(fmt.Sprintf("## US-%03d\n\n", i+1))
			sb.WriteString(fmt.Sprintf("%v\n\n", s))
		}
	}

	return sb.String()
}

// formatPRDDocument 格式化 PRD 文档
func formatPRDDocument(output map[string]any) string {
	var sb strings.Builder
	sb.WriteString("# 产品需求文档 (PRD)\n\n")

	// 提取 prd 字段，如果不存在则使用整个 output
	prdData, ok := output["prd"].(map[string]any)
	if !ok {
		// 尝试直接使用 output
		prdData = output
	}

	// 背景/概述
	if bg, ok := prdData["background"].(string); ok && bg != "" {
		sb.WriteString("## 背景\n\n")
		sb.WriteString(bg)
		sb.WriteString("\n\n")
	}

	// 目标
	if goal, ok := prdData["goal"].(string); ok && goal != "" {
		sb.WriteString("## 目标\n\n")
		sb.WriteString(goal)
		sb.WriteString("\n\n")
	}

	// 功能描述
	if features, ok := prdData["features"].([]interface{}); ok {
		sb.WriteString("## 功能描述\n\n")
		for i, f := range features {
			sb.WriteString(fmt.Sprintf("### 功能 %d\n\n", i+1))
			sb.WriteString(fmt.Sprintf("%v\n\n", f))
		}
	} else if features, ok := prdData["功能描述"].(map[string]any); ok {
		sb.WriteString("## 功能描述\n\n")
		for featName, featData := range features {
			sb.WriteString(fmt.Sprintf("### %s\n\n", featName))
			if featInfo, ok := featData.(map[string]any); ok {
				if desc, ok := featInfo["描述"].(string); ok {
					sb.WriteString(desc)
					sb.WriteString("\n\n")
				}
			} else {
				sb.WriteString(fmt.Sprintf("%v\n\n", featData))
			}
		}
	}

	// 验收标准
	if criteria, ok := prdData["acceptance_criteria"].([]interface{}); ok {
		sb.WriteString("## 验收标准\n\n")
		for _, c := range criteria {
			sb.WriteString(fmt.Sprintf("- [ ] %v\n", c))
		}
		sb.WriteString("\n")
	} else if criteria, ok := prdData["验收标准"].([]interface{}); ok {
		sb.WriteString("## 验收标准\n\n")
		for _, c := range criteria {
			sb.WriteString(fmt.Sprintf("- [ ] %v\n", c))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// DeveloperAdapter 开发者输出适配器
type DeveloperAdapter struct{}

func (a *DeveloperAdapter) Adapt(task protocol.Task, output map[string]any) ([]artifact.Artifact, error) {
	taskType := task.Type
	var artifacts []artifact.Artifact

	switch taskType {
	case "design_system":
		artifacts = a.adaptDesignOutput(output)
	case "implement_feature":
		artifacts = a.adaptDevelopOutput(output)
	case "fix_bug":
		artifacts = a.adaptFixOutput(output)
	default:
		// 通用处理
		artifacts = append(artifacts, artifact.NewDataArtifact(
			"output.json",
			output,
		))
	}

	return artifacts, nil
}

func (a *DeveloperAdapter) adaptDesignOutput(output map[string]any) []artifact.Artifact {
	var artifacts []artifact.Artifact

	// 1. 设计文档 - 转换为 Markdown
	// 优先从 output["design"] 提取，如果不存在则使用整个 output
	designContent := formatDesignDocument(output)
	artifacts = append(artifacts, artifact.NewMarkdownArtifact(
		"design.md",
		designContent,
	))

	// 2. 架构描述
	if arch, ok := output["architecture"].(string); ok && arch != "" {
		content := fmt.Sprintf("# 系统架构\n\n%s", arch)
		artifacts = append(artifacts, artifact.NewMarkdownArtifact(
			"architecture.md",
			content,
		))
	}

	// 3. 技术栈
	if techStack, ok := output["tech_stack"].([]interface{}); ok {
		content := formatTechStack(techStack)
		artifacts = append(artifacts, artifact.NewMarkdownArtifact(
			"tech-stack.md",
			content,
		))
	}

	// 4. API 定义 (如果有)
	if api, ok := output["api_definition"].(string); ok && api != "" {
		artifacts = append(artifacts, artifact.Artifact{
			Type:    artifact.TypeConfig,
			Path:    "api/openapi.yaml",
			Content: api,
		})
	}

	// 5. 元数据备份
	artifacts = append(artifacts, artifact.NewDataArtifact(
		"metadata.json",
		output,
	))

	return artifacts
}

func (a *DeveloperAdapter) adaptDevelopOutput(output map[string]any) []artifact.Artifact {
	var artifacts []artifact.Artifact

	// 1. 源代码 -> 写入 src/ 目录
	if code, ok := output["code"].(string); ok && code != "" {
		artifacts = append(artifacts, artifact.NewCodeArtifact(
			"src/main.go",
			code,
			map[string]any{"language": "go"},
		))
	}

	// 2. 测试代码 -> 写入 tests/ 目录
	if tests, ok := output["tests"].(string); ok && tests != "" {
		artifacts = append(artifacts, artifact.NewTestArtifact(
			"tests/main_test.go",
			tests,
		))
	}

	// 3. 文档 -> 写入 docs/ 目录
	if docs, ok := output["docs"].(string); ok && docs != "" {
		artifacts = append(artifacts, artifact.NewMarkdownArtifact(
			"docs/README.md",
			docs,
		))
	}

	// 4. go.mod (如果有依赖信息)
	if deps, ok := output["dependencies"].([]interface{}); ok && len(deps) > 0 {
		gomod := generateGoMod("project", deps)
		artifacts = append(artifacts, artifact.NewCodeArtifact(
			"src/go.mod",
			gomod,
			nil,
		))
	}

	// 5. 元数据备份
	artifacts = append(artifacts, artifact.NewDataArtifact(
		"metadata.json",
		output,
	))

	return artifacts
}

func (a *DeveloperAdapter) adaptFixOutput(output map[string]any) []artifact.Artifact {
	var artifacts []artifact.Artifact

	// Bug 修复补丁
	if code, ok := output["fixed_code"].(string); ok && code != "" {
		artifacts = append(artifacts, artifact.NewCodeArtifact(
			"src/main.go",
			code,
			nil,
		))
	}

	// 修改说明
	if changes, ok := output["changes"].([]interface{}); ok {
		content := formatChangeLog(changes)
		artifacts = append(artifacts, artifact.NewMarkdownArtifact(
			"CHANGELOG.md",
			content,
		))
	}

	return artifacts
}

// TesterAdapter 测试工程师输出适配器
type TesterAdapter struct{}

func (a *TesterAdapter) Adapt(task protocol.Task, output map[string]any) ([]artifact.Artifact, error) {
	taskType := task.Type
	var artifacts []artifact.Artifact

	switch taskType {
	case "write_test_plan":
		artifacts = a.adaptTestPlanOutput(output)
	case "execute_test":
		artifacts = a.adaptTestReportOutput(output)
	default:
		artifacts = append(artifacts, artifact.NewDataArtifact(
			"output.json",
			output,
		))
	}

	return artifacts, nil
}

func (a *TesterAdapter) adaptTestPlanOutput(output map[string]any) []artifact.Artifact {
	var artifacts []artifact.Artifact

	// 1. 测试用例文档
	if cases, ok := output["test_cases"].([]interface{}); ok {
		content := formatTestCases(cases)
		artifacts = append(artifacts, artifact.NewMarkdownArtifact(
			"test-plan.md",
			content,
		))
	}

	// 2. 测试脚本 (如果有)
	if script, ok := output["test_script"].(string); ok && script != "" {
		artifacts = append(artifacts, artifact.NewTestArtifact(
			"e2e/test_suite.go",
			script,
		))
	}

	// 3. 元数据
	artifacts = append(artifacts, artifact.NewDataArtifact(
		"metadata.json",
		output,
	))

	return artifacts
}

func (a *TesterAdapter) adaptTestReportOutput(output map[string]any) []artifact.Artifact {
	var artifacts []artifact.Artifact

	// 1. 测试报告 Markdown
	content := formatTestReport(output)
	artifacts = append(artifacts, artifact.NewMarkdownArtifact(
		"test-report.md",
		content,
	))

	// 2. Bug 列表
	if bugs, ok := output["bugs"].([]interface{}); ok && len(bugs) > 0 {
		bugContent := formatBugList(bugs)
		artifacts = append(artifacts, artifact.NewMarkdownArtifact(
			"bug-list.md",
			bugContent,
		))
	}

	// 3. 元数据
	artifacts = append(artifacts, artifact.NewDataArtifact(
		"metadata.json",
		output,
	))

	return artifacts
}

// 辅助函数

func extractMarkdownContent(data map[string]any, key, title string) string {
	val, ok := data[key]
	if !ok {
		return fmt.Sprintf("# %s\n\n暂无内容", title)
	}

	switch v := val.(type) {
	case string:
		return v
	case map[string]any:
		// 嵌套对象转 Markdown
		return convertMapToMarkdown(v, title, 1)
	default:
		return fmt.Sprintf("# %s\n\n%v", title, v)
	}
}

func convertMapToMarkdown(data map[string]any, title string, level int) string {
	var sb strings.Builder
	prefix := strings.Repeat("#", level)

	if title != "" {
		sb.WriteString(fmt.Sprintf("%s %s\n\n", strings.Repeat("#", level-1), title))
	}

	for key, val := range data {
		switch v := val.(type) {
		case string:
			if len(v) > 100 {
				sb.WriteString(fmt.Sprintf("%s %s\n\n%s\n\n", prefix, key, v))
			} else {
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", key, v))
			}
		case map[string]any:
			sb.WriteString(fmt.Sprintf("%s %s\n\n", prefix, key))
			sb.WriteString(convertMapToMarkdown(v, "", level+1))
		case []interface{}:
			sb.WriteString(fmt.Sprintf("%s %s\n\n", prefix, key))
			for _, item := range v {
				sb.WriteString(fmt.Sprintf("- %v\n", item))
			}
			sb.WriteString("\n")
		default:
			sb.WriteString(fmt.Sprintf("- **%s**: %v\n", key, v))
		}
	}

	return sb.String()
}

func formatTechStack(stack []interface{}) string {
	var sb strings.Builder
	sb.WriteString("# 技术栈\n\n")
	for _, tech := range stack {
		sb.WriteString(fmt.Sprintf("- %v\n", tech))
	}
	return sb.String()
}

func generateGoMod(projectName string, deps []interface{}) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("module %s\n\ngo 1.21\n\nrequire (\n", projectName))
	for _, dep := range deps {
		sb.WriteString(fmt.Sprintf("\t%v\n", dep))
	}
	sb.WriteString(")\n")
	return sb.String()
}

func formatChangeLog(changes []interface{}) string {
	var sb strings.Builder
	sb.WriteString("# 修改记录\n\n")
	for i, change := range changes {
		sb.WriteString(fmt.Sprintf("%d. %v\n", i+1, change))
	}
	return sb.String()
}

func formatTestCases(cases []interface{}) string {
	var sb strings.Builder
	sb.WriteString("# 测试计划\n\n")
	for _, c := range cases {
		switch tc := c.(type) {
		case map[string]interface{}:
			sb.WriteString(fmt.Sprintf("## %s: %s\n\n", tc["id"], tc["title"]))
			sb.WriteString(fmt.Sprintf("- **优先级**: %v\n", tc["priority"]))
			sb.WriteString(fmt.Sprintf("- **类型**: %v\n", tc["type"]))
			if steps, ok := tc["steps"].([]interface{}); ok {
				sb.WriteString("- **步骤**:\n")
				for _, step := range steps {
					sb.WriteString(fmt.Sprintf("  1. %v\n", step))
				}
			}
			sb.WriteString(fmt.Sprintf("- **预期结果**: %v\n\n", tc["expected"]))
		default:
			sb.WriteString(fmt.Sprintf("- %v\n", tc))
		}
	}
	return sb.String()
}

func formatTestReport(output map[string]any) string {
	var sb strings.Builder
	sb.WriteString("# 测试报告\n\n")

	if report, ok := output["report"].(map[string]interface{}); ok {
		sb.WriteString("## 执行统计\n\n")
		sb.WriteString(fmt.Sprintf("- **总用例数**: %v\n", report["total"]))
		sb.WriteString(fmt.Sprintf("- **通过**: %v\n", report["passed"]))
		sb.WriteString(fmt.Sprintf("- **失败**: %v\n", report["failed"]))
		if skipped, ok := report["skipped"]; ok {
			sb.WriteString(fmt.Sprintf("- **跳过**: %v\n", skipped))
		}
		sb.WriteString("\n")
	}

	if passed, ok := output["passed"].(bool); ok {
		if passed {
			sb.WriteString("## 结论\n\n✅ **测试通过**\n")
		} else {
			sb.WriteString("## 结论\n\n❌ **测试未通过**\n")
		}
	}

	return sb.String()
}

func formatBugList(bugs []interface{}) string {
	var sb strings.Builder
	sb.WriteString("# Bug 列表\n\n")
	for _, b := range bugs {
		switch bug := b.(type) {
		case map[string]interface{}:
			sb.WriteString(fmt.Sprintf("## %s\n\n", bug["id"]))
			sb.WriteString(fmt.Sprintf("- **严重程度**: %v\n", bug["severity"]))
			sb.WriteString(fmt.Sprintf("- **描述**: %v\n", bug["desc"]))
			if related, ok := bug["related_case"]; ok {
				sb.WriteString(fmt.Sprintf("- **相关用例**: %v\n", related))
			}
			sb.WriteString("\n")
		default:
			sb.WriteString(fmt.Sprintf("- %v\n", bug))
		}
	}
	return sb.String()
}

// formatDesignDocument 格式化设计文档
func formatDesignDocument(output map[string]any) string {
	var sb strings.Builder
	sb.WriteString("# 系统设计文档\n\n")

	// 提取 design 字段，如果不存在则使用整个 output
	designData, ok := output["design"].(map[string]any)
	if !ok {
		designData = output
	}

	// 系统概述
	if overview, ok := designData["overview"].(string); ok && overview != "" {
		sb.WriteString("## 系统概述\n\n")
		sb.WriteString(overview)
		sb.WriteString("\n\n")
	} else if systemOverview, ok := designData["系统概述"].(string); ok && systemOverview != "" {
		sb.WriteString("## 系统概述\n\n")
		sb.WriteString(systemOverview)
		sb.WriteString("\n\n")
	}

	// 模块划分
	if modules, ok := designData["module_division"].(map[string]any); ok {
		sb.WriteString("## 模块划分\n\n")
		for modName, modData := range modules {
			sb.WriteString(fmt.Sprintf("### %s\n\n", modName))
			if modInfo, ok := modData.(map[string]any); ok {
				if desc, ok := modInfo["description"].(string); ok {
					sb.WriteString(fmt.Sprintf("**描述**: %s\n\n", desc))
				}
				if files, ok := modInfo["files"].([]interface{}); ok {
					sb.WriteString("**文件列表**:\n")
					for _, f := range files {
						sb.WriteString(fmt.Sprintf("- `%v`\n", f))
					}
					sb.WriteString("\n")
				}
			}
		}
	} else if modules, ok := designData["模块划分"].(map[string]any); ok {
		sb.WriteString("## 模块划分\n\n")
		for modName, modData := range modules {
			sb.WriteString(fmt.Sprintf("### %s\n\n", modName))
			if modInfo, ok := modData.(map[string]any); ok {
				if desc, ok := modInfo["职责"].(string); ok {
					sb.WriteString(fmt.Sprintf("**职责**: %s\n\n", desc))
				}
				if submods, ok := modInfo["子模块"].([]interface{}); ok {
					sb.WriteString("**子模块**:\n")
					for _, sm := range submods {
						sb.WriteString(fmt.Sprintf("- %v\n", sm))
					}
					sb.WriteString("\n")
				}
			}
		}
	}

	// 接口定义
	if api, ok := designData["interface_definition"].(map[string]any); ok {
		sb.WriteString("## 接口定义\n\n")
		for apiName, apiData := range api {
			sb.WriteString(fmt.Sprintf("### %s\n\n", apiName))
			if apiInfo, ok := apiData.(map[string]any); ok {
				for method, methodData := range apiInfo {
					sb.WriteString(fmt.Sprintf("#### %s\n\n", method))
					if methodInfo, ok := methodData.(map[string]any); ok {
						if methodType, ok := methodInfo["method"].(string); ok {
							sb.WriteString(fmt.Sprintf("- **方法**: `%s`\n", methodType))
						}
						if req, ok := methodInfo["request"].(map[string]any); ok {
							sb.WriteString("- **请求参数**:\n")
							for k, v := range req {
								sb.WriteString(fmt.Sprintf("  - `%s`: %v\n", k, v))
							}
						}
						if resp, ok := methodInfo["response"].(map[string]any); ok {
							sb.WriteString("- **响应**:\n")
							for k, v := range resp {
								sb.WriteString(fmt.Sprintf("  - `%s`: %v\n", k, v))
							}
						}
					}
					sb.WriteString("\n")
				}
			}
		}
	} else if api, ok := designData["接口定义"].(map[string]any); ok {
		sb.WriteString("## 接口定义\n\n")
		for svcName, svcData := range api {
			sb.WriteString(fmt.Sprintf("### %s\n\n", svcName))
			if svcInfo, ok := svcData.(map[string]any); ok {
				for method, methodData := range svcInfo {
					sb.WriteString(fmt.Sprintf("#### %s\n\n", method))
					if methodInfo, ok := methodData.(map[string]any); ok {
						if m, ok := methodInfo["方法"].(string); ok {
							sb.WriteString(fmt.Sprintf("- **方法**: `%s`\n", m))
						}
						if req, ok := methodInfo["请求"].(map[string]any); ok {
							sb.WriteString("- **请求参数**:\n")
							for k, v := range req {
								sb.WriteString(fmt.Sprintf("  - `%s`: %v\n", k, v))
							}
						}
						if resp, ok := methodInfo["响应"].(map[string]any); ok {
							sb.WriteString("- **响应**:\n")
							for k, v := range resp {
								sb.WriteString(fmt.Sprintf("  - `%s`: %v\n", k, v))
							}
						}
					}
					sb.WriteString("\n")
				}
			}
		}
	}

	// 数据模型
	if dataModel, ok := designData["data_model"].(map[string]any); ok {
		sb.WriteString("## 数据模型\n\n")
		for entityName, entityData := range dataModel {
			sb.WriteString(fmt.Sprintf("### %s\n\n", entityName))
			if entityInfo, ok := entityData.(map[string]any); ok {
				if fields, ok := entityInfo["fields"].(map[string]any); ok {
					sb.WriteString("**字段**:\n")
					for fieldName, fieldType := range fields {
						sb.WriteString(fmt.Sprintf("- `%s`: %v\n", fieldName, fieldType))
					}
					sb.WriteString("\n")
				}
				if indexes, ok := entityInfo["indexes"].([]interface{}); ok {
					sb.WriteString("**索引**:\n")
					for _, idx := range indexes {
						sb.WriteString(fmt.Sprintf("- %v\n", idx))
					}
					sb.WriteString("\n")
				}
			}
		}
	} else if dataModel, ok := designData["数据模型"].(map[string]any); ok {
		sb.WriteString("## 数据模型\n\n")
		for entityName, entityData := range dataModel {
			sb.WriteString(fmt.Sprintf("### %s\n\n", entityName))
			if entityInfo, ok := entityData.(map[string]any); ok {
				if fields, ok := entityInfo["fields"].(map[string]any); ok {
					sb.WriteString("**字段**:\n")
					for fieldName, fieldType := range fields {
						sb.WriteString(fmt.Sprintf("- `%s`: %v\n", fieldName, fieldType))
					}
					sb.WriteString("\n")
				}
			}
		}
	}

	return sb.String()
}

// Factory 创建对应角色的 Adapter
func Factory(role string) StaffOutputAdapter {
	switch role {
	case "product":
		return &ProductAdapter{}
	case "developer":
		return &DeveloperAdapter{}
	case "tester":
		return &TesterAdapter{}
	default:
		return nil
	}
}
