package meeting

import (
	"cyberteam/internal/llm"
	"cyberteam/internal/profile"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Participant Staff 参与者接口
type Participant interface {
	GetRole() string
	GetName() string
	GetProfile() *profile.Profile
	GenerateResponse(ctx *DiscussionContext) (string, error)
}

// DiscussionContext 讨论上下文
type DiscussionContext struct {
	MeetingID    string
	Topic        string
	History      []Message
	LastMessage  *Message
	Mentioned    bool // 是否被@
	Mode         Mode
	Transcript   string
}

// StaffParticipant Staff 参与者实现
type StaffParticipant struct {
	Role      string
	Name      string
	Profile   *profile.Profile
	LLMClient llm.Client
	Model     string
}

func (s *StaffParticipant) GetRole() string {
	return s.Role
}

func (s *StaffParticipant) GetName() string {
	return s.Name
}

func (s *StaffParticipant) GetProfile() *profile.Profile {
	return s.Profile
}

// GenerateResponse 生成回复
func (s *StaffParticipant) GenerateResponse(ctx *DiscussionContext) (string, error) {
	// 构建 prompt
	prompt := s.buildPrompt(ctx)
	
	systemPrompt := s.Profile.BuildSystemPrompt("discuss")
	if systemPrompt == "" {
		systemPrompt = s.buildDefaultSystemPrompt()
	}
	
	resp, err := s.LLMClient.Complete([]llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}, &llm.CompleteOptions{
		Model:       s.Model,
		Temperature: 0.7,
		MaxTokens:   500,
	})
	
	if err != nil {
		return "", err
	}
	
	return resp.Content, nil
}

func (s *StaffParticipant) buildPrompt(ctx *DiscussionContext) string {
	var sb strings.Builder
	
	sb.WriteString(fmt.Sprintf("会议主题: %s\n\n", ctx.Topic))
	sb.WriteString(fmt.Sprintf("讨论模式: %s\n", ctx.Mode))
	
	if ctx.Transcript != "" {
		sb.WriteString("\n=== 之前的讨论 ===\n")
		// 只保留最近 10 条消息
		lines := strings.Split(ctx.Transcript, "\n")
		if len(lines) > 10 {
			lines = lines[len(lines)-10:]
		}
		sb.WriteString(strings.Join(lines, "\n"))
		sb.WriteString("\n===================\n\n")
	}
	
	if ctx.LastMessage != nil {
		if ctx.Mentioned {
			sb.WriteString(fmt.Sprintf("你被 @%s 点名发言: %s\n\n", s.Name, ctx.LastMessage.From))
			sb.WriteString("请直接回复这个问题，给出你的专业意见。")
		} else {
			sb.WriteString(fmt.Sprintf("上一条消息来自 %s: %s\n\n", ctx.LastMessage.From, ctx.LastMessage.Content))
			sb.WriteString("作为 " + s.Name + "，请参与讨论。可以：\n")
			sb.WriteString("1. 发表你的观点\n")
			sb.WriteString("2. 回复某人的观点\n")
			sb.WriteString("3. 提出问题\n")
			sb.WriteString("4. 如果没有新观点，可以简单说\"暂无疑问\"\n")
		}
	}
	
	sb.WriteString("\n请用第一人称发言，简短有力（50-200字）。")
	
	return sb.String()
}

func (s *StaffParticipant) buildDefaultSystemPrompt() string {
	return fmt.Sprintf(`你是%s，%s。

你在一个团队会议中参与讨论。请遵守以下原则：
1. 简短直接，不绕弯子
2. 专业但有亲和力
3. 可以质疑，但要建设性
4. 如果不确定，诚实说明

回复格式：直接输出你的发言内容，不要加引号或其他格式。`, s.Name, s.Profile.Description)
}

// Facilitator 会议主持人（Boss 角色）
type Facilitator struct {
	Room      *Room
	LLMClient llm.Client
	Model     string
}

// GenerateSummary 生成会议总结
func (f *Facilitator) GenerateSummary(meeting *Meeting) (string, []string, error) {
	transcript := meeting.GetTranscript()
	
	prompt := fmt.Sprintf(`请总结以下会议内容：

主题: %s
参与者: %s

聊天记录:
%s

请输出 JSON 格式:
{
  "summary": "会议总结（200字以内）",
  "action_items": ["行动项1", "行动项2", ...]
}`, meeting.Topic, strings.Join(meeting.Participants, ", "), transcript)

	resp, err := f.LLMClient.Complete([]llm.Message{
		{Role: "system", Content: "你是会议主持人，擅长提炼关键信息和行动项。"},
		{Role: "user", Content: prompt},
	}, &llm.CompleteOptions{
		Model:       f.Model,
		Temperature: 0.3,
		MaxTokens:   1000,
	})
	
	if err != nil {
		return "", nil, err
	}
	
	var result struct {
		Summary     string   `json:"summary"`
		ActionItems []string `json:"action_items"`
	}
	
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		// 解析失败，返回原始内容作为总结
		return resp.Content, []string{}, nil
	}
	
	return result.Summary, result.ActionItems, nil
}

// DecideNextSpeaker 决定下一个发言者（轮流模式）
func (f *Facilitator) DecideNextSpeaker(meeting *Meeting, lastSpeaker string) string {
	participants := meeting.Participants
	if len(participants) == 0 {
		return ""
	}
	
	// 找到上一个发言者的位置
	idx := -1
	for i, p := range participants {
		if p == lastSpeaker {
			idx = i
			break
		}
	}
	
	// 下一个
	nextIdx := (idx + 1) % len(participants)
	return participants[nextIdx]
}

// ShouldContinue 判断会议是否应该继续
func (f *Facilitator) ShouldContinue(meeting *Meeting) bool {
	// 简单规则：会议进行超过 30 分钟或消息超过 50 条
	duration := time.Since(meeting.StartedAt)
	if duration > 30*time.Minute {
		return false
	}
	if len(meeting.Messages) > 50 {
		return false
	}
	return true
}