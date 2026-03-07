package common

import (
	"fmt"
	"os"

	"cyberteam/internal/workflow"
)

// DebugMode 全局 debug 开关
var DebugMode = false

// Debugf 在 debug 模式下打印消息到 stderr
// 用法: common.Debugf("[Boss] message: %v\n", value)
func Debugf(format string, args ...interface{}) {
	if DebugMode {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}

// GetStageNumber 根据阶段获取阶段编号
func GetStageNumber(stage workflow.Stage) int {
	stages := map[workflow.Stage]int{
		workflow.StageRequirement: 1,
		workflow.StageDesign:      2,
		workflow.StageReview:      3,
		workflow.StageDevelop:     4,
		workflow.StageTest:        5,
		workflow.StageDeploy:      6,
	}
	if n, ok := stages[stage]; ok {
		return n
	}
	return 0
}

// GetStageDirName 根据阶段编号获取目录名
func GetStageDirName(stageNum int) string {
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

// GetStageName 根据阶段获取中文名称
func GetStageName(stage workflow.Stage) string {
	names := map[workflow.Stage]string{
		workflow.StageRequirement: "需求分析",
		workflow.StageDesign:      "系统设计",
		workflow.StageReview:      "设计评审",
		workflow.StageDevelop:     "功能开发",
		workflow.StageTest:        "测试验证",
		workflow.StageDeploy:      "部署上线",
	}
	if name, ok := names[stage]; ok {
		return name
	}
	return string(stage)
}

// GetRoleForStage 根据阶段获取角色
func GetRoleForStage(stage workflow.Stage) string {
	roles := map[workflow.Stage]string{
		workflow.StageRequirement: "product",
		workflow.StageDesign:      "developer",
		workflow.StageReview:      "product",
		workflow.StageDevelop:     "developer",
		workflow.StageTest:        "tester",
		workflow.StageDeploy:      "developer",
	}
	if role, ok := roles[stage]; ok {
		return role
	}
	return ""
}

// GetRoleIcon 根据角色获取图标
func GetRoleIcon(role string) string {
	icons := map[string]string{
		"product":   "📝",
		"developer": "💻",
		"tester":    "🧪",
	}
	if icon, ok := icons[role]; ok {
		return icon
	}
	return "👤"
}

// GetStatusIcon 根据状态获取图标
func GetStatusIcon(status workflow.Status) string {
	icons := map[workflow.Status]string{
		workflow.StatusPending:    "⏳",
		workflow.StatusAssigned:   "👤",
		workflow.StatusInProgress: "🔄",
		workflow.StatusCompleted:  "✅",
		workflow.StatusRejected:   "❌",
		workflow.StatusFailed:     "💥",
	}
	if icon, ok := icons[status]; ok {
		return icon
	}
	return "❓"
}

// Truncate 截断字符串（按 Unicode 字符数，安全处理中文）
func Truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}
