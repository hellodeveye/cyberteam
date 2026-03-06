package staffutil

import (
	"cyberteam/internal/llm"
	"cyberteam/internal/profile"
	"cyberteam/internal/tools"
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
	// 先尝试自动执行工具（如果是技术问题）
	if mentioned && (p.Role == "developer" || p.Role == "tester") {
		if toolResult, executed := p.AutoExecuteTool(content, "/tmp"); executed {
			// 工具执行成功，构建带工具结果的回复
			return p.buildToolReply(content, toolResult)
		}
	}

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
		MaxTokens:   300,
	})

	if err != nil {
		return p.fallbackReply(mentioned)
	}

	return resp.Content
}

// buildToolReply 构建带工具结果的回复
func (p *MeetingParticipant) buildToolReply(question, toolResult string) string {
	return fmt.Sprintf("我来验证一下...\n%s", toolResult)
}

func (p *MeetingParticipant) buildSystemPrompt() string {
	return fmt.Sprintf(`你是%s，%s。

你现在在一个团队会议中参与讨论。

**重要原则：**
1. 先判断用户意图，简单问候就简单回应，不要过度展开
2. 只有当讨论具体技术/业务问题时，才展现你的专业特长
3. 简短直接（30-50字），不要长篇大论
4. 可以质疑，但要建设性
5. 如果没有新观点，简单说"同意"或"没问题"

回复格式：直接输出你的发言内容，不要加引号或其他格式。`, p.Name, p.Profile.Description)
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

	if mentioned {
		sb.WriteString(fmt.Sprintf("你被 @%s 点名发言\n", from))
		sb.WriteString(fmt.Sprintf("问题: %s\n\n", content))
		sb.WriteString("请直接回复这个问题，给出你的专业意见。")
	} else {
		sb.WriteString(fmt.Sprintf("上一条消息来自 %s: %s\n\n", from, content))
		sb.WriteString("作为 " + p.Name + "，请参与讨论。可以：\n")
		sb.WriteString("1. 发表你的观点\n")
		sb.WriteString("2. 回复某人的观点\n")
		sb.WriteString("3. 提出问题\n")
		sb.WriteString("4. 如果没有新观点，可以简单说\"同意\"或\"暂无疑问\"\n")
	}

	return sb.String()
}

// AutoExecuteTool 自动检测并执行工具
func (p *MeetingParticipant) AutoExecuteTool(question string, workDir string) (string, bool) {
	// 创建 bash 工具
	bashTool := tools.NewBashTool(workDir)

	// 检测需要执行什么命令
	cmd := p.detectCommand(question)
	if cmd == "" {
		return "", false
	}

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
	if matched, _ := regexp.MatchString(`(进程|process|ps|运行中)`, q); matched {
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