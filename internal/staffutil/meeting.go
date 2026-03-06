package staffutil

import (
	"cyberteam/internal/llm"
	"cyberteam/internal/profile"
	"cyberteam/internal/tools"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// MeetingParticipant Staff 会议参与者
type MeetingParticipant struct {
	Role      string
	Name      string
	Profile   *profile.Profile
	LLMClient llm.Client
	Model     string
	MCPClient *MCPClient // MCP 客户端
}

// NewMeetingParticipant 创建会议参与者
func NewMeetingParticipant(role, name string, profile *profile.Profile, llmClient llm.Client, model string) *MeetingParticipant {
	return &MeetingParticipant{
		Role:      role,
		Name:      name,
		Profile:   profile,
		LLMClient: llmClient,
		Model:     model,
	}
}

// GenerateReply 生成会议回复（支持自动工具执行）
func (p *MeetingParticipant) GenerateReply(meetingID, topic, transcript, from, content string, mentioned bool) string {
	// 构建系统提示
	systemPrompt := p.buildSystemPrompt()

	// 构建用户提示
	userPrompt := p.buildUserPrompt(topic, transcript, from, content, mentioned)

	// 调用 LLM
	resp, err := p.LLMClient.Complete([]llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, &llm.CompleteOptions{
		Model:       p.Model,
		Temperature: 0.7,
		MaxTokens:   500,
	})

	if err != nil {
		return p.fallbackReply(mentioned)
	}

	reply := resp.Content

	// 检查是否需要调用 MCP 工具
	if p.MCPClient != nil && mentioned {
		if toolName, args, ok := parseToolCall(reply); ok {
			result, err := p.MCPClient.CallTool(toolName, args)
			if err == nil && result != "" {
				// 将工具结果附加到回复中
				return fmt.Sprintf("%s\n\n[工具查询结果]\n%s", reply, result)
			}
		}
	}

	// 回退到本地 bash 工具
	if mentioned && (p.Role == "developer" || p.Role == "tester") {
		if toolResult, executed := p.AutoExecuteTool(content, "/tmp"); executed {
			return p.buildToolReply(content, toolResult)
		}
	}

	return reply
}

// parseToolCall 解析 LLM 回复中的工具调用
// 格式: [TOOL:server:tool]{json_args}
func parseToolCall(reply string) (string, map[string]interface{}, bool) {
	re := regexp.MustCompile(`\[TOOL:([^\]]+)\]\s*(\{[^\}]+\})`)
	matches := re.FindStringSubmatch(reply)
	if len(matches) < 3 {
		return "", nil, false
	}

	toolName := matches[1]
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(matches[2]), &args); err != nil {
		return "", nil, false
	}

	return toolName, args, true
}

// buildToolReply 构建带工具结果的回复
func (p *MeetingParticipant) buildToolReply(question, toolResult string) string {
	return fmt.Sprintf("我来验证一下...\n%s", toolResult)
}

func (p *MeetingParticipant) buildSystemPrompt() string {
	toolsInfo := ""
	if p.MCPClient != nil {
		toolsInfo = p.MCPClient.GetToolsPrompt()
	}

	return fmt.Sprintf(`你是%s，%s。

团队成员：
- Kai：团队负责人（Boss）
- Sarah：产品经理
- Alex：开发工程师
- Mia：测试工程师

%s

你现在正在团队会议中，保持放松的状态，像平时聊天一样自然交流。

**交流风格：**
- 直接、真诚，不绕弯子
- 简短有力，像日常对话
- 根据对方身份调整语气（对Boss尊重但不必拘谨，对同事随意）

回复格式：直接输出你的发言内容，不要加引号或其他格式。`, p.Name, p.Profile.Description, toolsInfo)
}

func (p *MeetingParticipant) buildUserPrompt(topic, transcript, from, content string, mentioned bool) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("会议主题: %s\n\n", topic))

	if transcript != "" {
		sb.WriteString("=== 之前的讨论 ===\n")
		// 只保留最近 10 条消息
		lines := strings.Split(transcript, "\n")
		if len(lines) > 10 {
			lines = lines[len(lines)-10:]
		}
		sb.WriteString(strings.Join(lines, "\n"))
		sb.WriteString("\n===================\n\n")
	}

	// 说明说话人身份
	speakerRole := "同事"
	switch from {
	case "Kai":
		speakerRole = "Boss"
	case "Sarah":
		speakerRole = "产品经理"
	case "Alex":
		speakerRole = "开发工程师"
	case "Mia":
		speakerRole = "测试工程师"
	}
	sb.WriteString(fmt.Sprintf("%s(%s): %s\n", from, speakerRole, content))
	if mentioned {
		sb.WriteString("你被点名了。")
	}

	return sb.String()
}

// AutoExecuteTool 自动检测并执行工具
func (p *MeetingParticipant) AutoExecuteTool(question string, workDir string) (string, bool) {
	// 检测需要执行什么命令
	cmd := p.detectCommand(question)
	if cmd == "" {
		return "", false
	}

	// 创建 bash 工具
	bashTool := tools.NewBashTool(workDir)

	// 执行命令
	result := bashTool.Execute(cmd)
	if !result.Success {
		return fmt.Sprintf("执行失败: %s", result.Error), true
	}

	// 截断输出（避免太长）
	output := result.Output
	if len(output) > 500 {
		output = output[:500] + "\n... (已截断)"
	}

	return fmt.Sprintf("执行结果:\n```\n%s\n```", output), true
}

// detectCommand 检测需要执行的命令
func (p *MeetingParticipant) detectCommand(question string) string {
	// 转换为小写方便匹配
	q := strings.ToLower(question)

	// 匹配 curl 请求
	if matched, _ := regexp.MatchString(`(访问|curl|http|网站|网址).*`, q); matched {
		// 提取 URL
		urlRegex := regexp.MustCompile(`(https?://[^\s]+|[^\s]+\.(com|cn|org|net|io)[^\s]*)`)
		if matches := urlRegex.FindStringSubmatch(question); len(matches) > 0 {
			url := matches[0]
			// 如果没有协议，添加 http
			if !strings.HasPrefix(url, "http") {
				url = "http://" + url
			}
			return fmt.Sprintf("curl -s -I -m 5 %s | head -20", url)
		}
	}

	// 匹配 ping 请求
	if matched, _ := regexp.MatchString(`(ping|连通性|通不通|能否连接)`, q); matched {
		hostRegex := regexp.MustCompile(`(baidu\.com|google\.com|github\.com|[\w\-]+\.(com|cn|org))`)
		if matches := hostRegex.FindStringSubmatch(question); len(matches) > 0 {
			return fmt.Sprintf("ping -c 1 -W 3 %s", matches[0])
		}
	}

	// 匹配文件/目录查询
	if matched, _ := regexp.MatchString(`(有什么文件|目录|列出|ls|文件列表)`, q); matched {
		return "ls -la"
	}

	// 匹配磁盘/空间查询
	if matched, _ := regexp.MatchString(`(磁盘|空间|df|容量)`, q); matched {
		return "df -h"
	}

	// 匹配内存查询
	if matched, _ := regexp.MatchString(`(内存|memory|free)`, q); matched {
		return "free -h"
	}

	// 匹配进程查询
	if matched, _ := regexp.MatchString(`(进程|process|\bps\b|运行中)`, q); matched {
		return "ps aux | head -10"
	}

	return ""
}

func (p *MeetingParticipant) fallbackReply(mentioned bool) string {
	replies := []string{
		"同意这个观点。",
		"我觉得可行。",
		"暂时没有问题。",
		"需要再考虑一下。",
	}
	if mentioned {
		replies = append(replies, "这个问题我需要深入研究一下。", "我认为技术上可行。")
	}
	// 根据角色调整
	switch p.Role {
	case "product":
		return "从用户角度看，这个方案不错。"
	case "developer":
		return "技术上可行，实现成本中等。"
	case "tester":
		return "测试覆盖没问题，可以考虑。"
	}
	return replies[0]
}

// tryMCPCall 尝试使用 MCP 工具
func (p *MeetingParticipant) tryMCPCall(question string) (string, bool) {
	if p.MCPClient == nil {
		return "", false
	}

	q := strings.ToLower(question)

	// 匹配 GitHub 查询
	if matched, _ := regexp.MatchString(`(github|开源|仓库|repo)`, q); matched {
		// 提取查询关键词
		keywords := extractKeywords(question)
		if keywords != "" {
			result, err := p.MCPClient.CallTool("github:search_repositories", map[string]interface{}{
				"query": keywords,
			})
			if err == nil && result != "" {
				return result, true
			}
		}
	}

	// 匹配网页抓取
	if matched, _ := regexp.MatchString(`(网站|官网|网页|文档|doc)`, q); matched {
		url := extractURL(question)
		if url != "" {
			result, err := p.MCPClient.CallTool("fetch:fetch_url", map[string]interface{}{
				"url": url,
			})
			if err == nil && result != "" {
				return result, true
			}
		}
	}

	return "", false
}

// extractKeywords 提取查询关键词
func extractKeywords(text string) string {
	// 简单实现：去掉常见词，返回剩余内容
	words := []string{"github", "开源", "仓库", "repo", "查", "找", "下", "一下"}
	result := text
	for _, w := range words {
		result = strings.ReplaceAll(result, w, "")
	}
	return strings.TrimSpace(result)
}

// extractURL 从文本提取 URL
func extractURL(text string) string {
	urlRegex := regexp.MustCompile(`(https?://[^\s]+)`)
	if matches := urlRegex.FindStringSubmatch(text); len(matches) > 0 {
		return matches[0]
	}
	return ""
}
