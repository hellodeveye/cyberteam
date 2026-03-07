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

// 角色颜色映射（UI 样式决策，与 Profile 无关，保持静态）
var roleColors = map[string]string{
	"product":   ColorGreen,  // 产品 - 绿色
	"developer": ColorBlue,   // 开发 - 蓝色
	"tester":    ColorYellow, // 测试 - 黄色
	"boss":      ColorPurple, // Boss - 紫色
}

// getBossName 从 gBossProfile 动态读取 Boss 名字
func getBossName() string {
	if gBossProfile != nil && gBossProfile.Name != "" {
		return gBossProfile.Name
	}
	return "Boss"
}

// getNameToRole 从运行时 Registry 实时构建 name→role 映射（惰性求值，无时序问题）
func getNameToRole() map[string]string {
	if gBoss == nil {
		return map[string]string{}
	}
	return gBoss.GetNameToRoleMap()
}

// getOnlineStaffNames 从运行时 Registry 获取在线 staff 名字列表
func getOnlineStaffNames() []string {
	if gBoss == nil {
		return nil
	}
	return gBoss.GetOnlineStaffNames()
}

// roleForName 查询 name 对应的 role；未找到时返回 ("", false)
func roleForName(name string) (string, bool) {
	r, ok := getNameToRole()[name]
	return r, ok
}

// colorizeMeetingReply 给会议回复添加颜色
func colorizeMeetingReply(content string) string {
	name := extractNameFromReply(content)
	role := ""
	if r, ok := roleForName(name); ok {
		role = r
	}
	if name == "boss" {
		role = "boss"
	}

	color := ColorWhite
	if c, ok := roleColors[role]; ok {
		color = c
	}

	// 去掉 [meetingID] 前缀
	cleanContent := content
	if idx := strings.Index(content, "] **"); idx != -1 {
		cleanContent = content[idx+2:]
	}

	if name != "" {
		oldName := fmt.Sprintf("**%s**", name)
		newName := fmt.Sprintf("%s%s%s%s", ColorBold, color, name, ColorReset)
		cleanContent = strings.Replace(cleanContent, oldName, newName, 1)
	}

	return cleanContent
}

// colorizeStaffMessage 给普通 Staff 消息添加颜色（从 Registry 查 role，退化到 staffID 前缀推断）
func colorizeStaffMessage(staffID, content string) string {
	role := roleByStaffID(staffID)
	color := ColorWhite
	if c, ok := roleColors[role]; ok {
		color = c
	}
	return fmt.Sprintf("%s[%s]%s %s", color, staffID[:8], ColorReset, content)
}

// roleByStaffID 从 staffID 推断 role。
// staffID 格式固定为 "<role>-<timestamp>"（见 manager.HireStaff），取第一段即可。
func roleByStaffID(staffID string) string {
	if idx := strings.Index(staffID, "-"); idx > 0 {
		return staffID[:idx]
	}
	return staffID
}

// extractNameFromReply 从回复中提取名字（格式: **Name**: content）
func extractNameFromReply(content string) string {
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
	nameToRole := getNameToRole()
	for _, w := range strings.Fields(content) {
		if strings.HasPrefix(w, "@") {
			name := strings.TrimPrefix(w, "@")
			if role, ok := nameToRole[name]; ok {
				mentions = append(mentions, role)
			} else {
				// 直接当作 role 使用（支持 @developer 这类写法）
				mentions = append(mentions, name)
			}
		}
	}
	return mentions
}

// getSenderColor 获取发送者的颜色
func getSenderColor(from string) string {
	role := ""
	if r, ok := roleForName(from); ok {
		role = r
	}
	if from == "boss" {
		role = "boss"
	}
	if c, ok := roleColors[role]; ok {
		return c
	}
	return ColorWhite
}
