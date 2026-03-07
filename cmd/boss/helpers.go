package main

import (
	"fmt"
	"regexp"
	"strings"
)

// ANSI 颜色代码
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorGray   = "\033[90m" // 灰色
	ColorBold   = "\033[1m"
)

// 角色颜色映射
var roleColors = map[string]string{
	"product":   ColorGreen,  // 产品 - 绿色
	"developer": ColorBlue,   // 开发 - 蓝色
	"tester":    ColorYellow, // 测试 - 黄色
	"boss":      ColorPurple, // Boss - 紫色
}

// 名字到角色的映射
var nameToRole = map[string]string{
	"Sarah": "product",
	"Alex":  "developer",
	"Mia":   "tester",
}

// colorizeMeetingReply 给会议回复添加颜色
func colorizeMeetingReply(content string) string {
	// 提取名字和角色
	// 格式: [meetingID] **Name**: content 或 **Name**: content
	name := extractNameFromReply(content)
	role := ""
	if r, ok := nameToRole[name]; ok {
		role = r
	}
	// 特殊处理 boss
	if name == "boss" {
		role = "boss"
	}

	// 获取颜色
	color := ColorWhite
	if c, ok := roleColors[role]; ok {
		color = c
	}

	// 去掉 [meetingID] 前缀
	cleanContent := content
	if idx := strings.Index(content, "] **"); idx != -1 {
		cleanContent = content[idx+2:] // 去掉 "] "
	}

	// 替换 **Name** 为带颜色的版本
	if name != "" {
		oldName := fmt.Sprintf("**%s**", name)
		newName := fmt.Sprintf("%s%s%s%s", ColorBold, color, name, ColorReset)
		cleanContent = strings.Replace(cleanContent, oldName, newName, 1)
	}

	return cleanContent
}

// colorizeStaffMessage 给普通 Staff 消息添加颜色
func colorizeStaffMessage(staffID, content string) string {
	// 从 staffID 推断角色 (如 "developer-xxx")
	role := ""
	if strings.HasPrefix(staffID, "product") {
		role = "product"
	} else if strings.HasPrefix(staffID, "developer") {
		role = "developer"
	} else if strings.HasPrefix(staffID, "tester") {
		role = "tester"
	}

	color := ColorWhite
	if c, ok := roleColors[role]; ok {
		color = c
	}

	return fmt.Sprintf("%s[%s]%s %s", color, staffID[:8], ColorReset, content)
}

// extractNameFromReply 从回复中提取名字
func extractNameFromReply(content string) string {
	// 匹配 **Name**: 格式
	re := regexp.MustCompile(`\*\*([^*]+)\*\*:`)
	matches := re.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// extractMentions 提取 @ 提及并转换为 role
func extractMentions(content string) []string {
	var mentions []string
	words := strings.Fields(content)
	for _, w := range words {
		if strings.HasPrefix(w, "@") {
			name := strings.TrimPrefix(w, "@")
			// 尝试转换为 role
			if role, ok := nameToRole[name]; ok {
				mentions = append(mentions, role)
			} else {
				// 直接使用（可能是 role 名称）
				mentions = append(mentions, name)
			}
		}
	}
	return mentions
}

// getSenderColor 获取发送者的颜色
func getSenderColor(from string) string {
	role := ""
	if r, ok := nameToRole[from]; ok {
		role = r
	}
	// 特殊处理 boss
	if from == "boss" {
		role = "boss"
	}
	if c, ok := roleColors[role]; ok {
		return c
	}
	return ColorWhite
}
