package staffutil

import (
	"cyberteam/internal/agent"
	"cyberteam/internal/llm"
	"cyberteam/internal/profile"
	"cyberteam/internal/tools"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// TeamMember represents a team member in the organization
type TeamMember struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

// MeetingParticipant Staff 会议参与者
type MeetingParticipant struct {
	Role        string
	Name        string
	Profile     *profile.Profile
	LLMClient   llm.Client
	Model       string
	MCPClient   *StaffMCPClient // MCP 客户端
	BashTool    *tools.BashTool // Bash 工具
	Memory      agent.Memory    // 记忆系统
	TeamMembers []TeamMember    // 团队成员列表（动态获取）
	Debug       bool            // Debug 模式
}

// NewMeetingParticipant 创建会议参与者
func NewMeetingParticipant(role, name string, profile *profile.Profile, llmClient llm.Client, model string, memory agent.Memory, debug bool) *MeetingParticipant {
	return &MeetingParticipant{
		Role:        role,
		Name:        name,
		Profile:     profile,
		LLMClient:   llmClient,
		Model:       model,
		Memory:      memory,
		TeamMembers: []TeamMember{},
		Debug:       debug,
	}
}

// SetTeamMembers 设置团队成员列表
func (p *MeetingParticipant) SetTeamMembers(members []TeamMember) {
	p.TeamMembers = members
}

// Debugf prints a debug message if debug mode is enabled
func (p *MeetingParticipant) Debugf(format string, args ...interface{}) {
	if p.Debug {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}

// GenerateReply 生成会议回复（使用 Agent）
func (p *MeetingParticipant) GenerateReply(meetingID, topic, transcript, from, content string, mentioned bool) string {
	// 构建系统提示
	systemPrompt := p.buildSystemPrompt()

	// 构建用户提示
	userPrompt := p.buildUserPrompt(topic, transcript, from, content, mentioned)

	// 创建 MCP 适配器
	var mcpExecutor agent.ToolExecutor
	if p.MCPClient != nil {
		mcpExecutor = &MCPAdapter{Client: p.MCPClient}
	}

	// 创建 Bash 适配器
	var bashExecutor agent.ToolExecutor
	if p.BashTool != nil {
		bashExecutor = &BashAdapter{Tool: p.BashTool}
	}

	// 创建 Agent (传入 Memory、MCP 和 Bash)
	coreAgent := agent.New(agent.Config{
		Name:         p.Name,
		Model:        p.Model,
		LLMClient:    p.LLMClient,
		MCPExecutor:  mcpExecutor,
		BashExecutor: bashExecutor,
		Memory:       p.Memory,
		SystemPrompt: systemPrompt,
		Debug:        p.Debug,
	})

	// 注册 MCP 工具到 Agent 的 ToolRegistry
	if p.MCPClient != nil {
		registry := coreAgent.ToolRegistry()
		mcpTools := p.MCPClient.ListTools()
		p.Debugf("[Agent] 注册 %d 个 MCP 工具\n", len(mcpTools))
		for _, t := range mcpTools {
			p.Debugf("[Agent]   - %s\n", t.Name)
			// 将 InputSchema (map) 转为 json.RawMessage
			var schema json.RawMessage
			if t.InputSchema != nil {
				schema, _ = json.Marshal(t.InputSchema)
			}
			registry.Register(agent.Tool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: schema,
				Executor:    "mcp",
			})
		}
	}

	// 注册 Bash 工具
	if p.BashTool != nil {
		registry := coreAgent.ToolRegistry()
		registry.Register(agent.Tool{
			Name:        "bash:execute",
			Description: "执行 bash 命令（如 ls, cat, go build, git status 等）",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"要执行的 bash 命令"}},"required":["command"]}`),
			Executor:    "bash",
		})
		p.Debugf("[Agent]   - bash:execute\n")
	}

	// 使用 Agent 执行（带上下文）
	result := coreAgent.ExecuteWithContext(transcript, userPrompt)
	p.Debugf("[Agent] 最终回复: %s\n", result)
	return result
}

func (p *MeetingParticipant) buildSystemPrompt() string {
	toolsInfo := ""
	if p.MCPClient != nil {
		toolsInfo = p.MCPClient.GetToolsPrompt()
	}

	// 构建团队成员列表
	teamInfo := p.buildTeamInfo()

	// 构建工具使用指南
	var toolsGuide strings.Builder
	if toolsInfo != "" {
		toolsGuide.WriteString(toolsInfo)
		toolsGuide.WriteString("\n\n")
	}
	toolsGuide.WriteString("**工具使用原则：**\n")
	toolsGuide.WriteString("- 优先使用 bash:execute 工具解决问题（查文件、看代码、执行命令、查系统信息等）\n")
	toolsGuide.WriteString("- 不要猜测或编造答案，能用工具验证的就用工具\n")

	return fmt.Sprintf(`你是%s，%s。

%s

%s

你现在正在团队会议中，保持放松的状态，像平时聊天一样自然交流。

**交流风格：**
- 直接、真诚，不绕弯子
- 简短有力，像日常对话
- 根据对方身份调整语气（对Boss尊重但不必拘谨，对同事随意）

回复格式：直接输出你的发言内容，不要加引号或其他格式。`, p.Name, p.Profile.Description, teamInfo, toolsGuide.String())
}

// buildTeamInfo 构建团队成员信息
func (p *MeetingParticipant) buildTeamInfo() string {
	if len(p.TeamMembers) == 0 {
		return "团队成员：（暂未获取到团队信息）"
	}

	var sb strings.Builder
	sb.WriteString("团队成员：\n")
	for _, m := range p.TeamMembers {
		sb.WriteString(fmt.Sprintf("- %s：%s\n", m.Name, m.Role))
	}
	return sb.String()
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
