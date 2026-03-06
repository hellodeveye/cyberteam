package meeting

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Mode 会议模式
type Mode string

const (
	ModeFree     Mode = "free"   // 自由讨论
	ModeRound    Mode = "round"  // 轮流发言
	ModeBossLead Mode = "boss"   // Boss 主导
	ModeSilent   Mode = "silent" // 静默模式（异步）
)

// Message 会议消息
type Message struct {
	ID        string         `json:"id"`
	From      string         `json:"from"` // 发送者: "boss", "product", "developer", "tester"
	Content   string         `json:"content"`
	Type      MsgType        `json:"type"` // text | mention | action
	MentionTo []string       `json:"mention_to,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type MsgType string

const (
	MsgText    MsgType = "text"
	MsgMention MsgType = "mention"
	MsgAction  MsgType = "action" // 系统动作: 会议开始、结束等
	MsgTyping  MsgType = "typing" // 正在输入...
)

// Meeting 会议
type Meeting struct {
	ID           string     `json:"id"`
	Topic        string     `json:"topic"`
	Mode         Mode       `json:"mode"`
	Status       Status     `json:"status"`
	Participants []string   `json:"participants"` // 参与者列表
	Messages     []Message  `json:"messages"`
	StartedAt    time.Time  `json:"started_at"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`
	Summary      string     `json:"summary,omitempty"`
	ActionItems  []string   `json:"action_items,omitempty"`
	CreatedBy    string     `json:"created_by"`
}

type Status string

const (
	StatusPending   Status = "pending"   // 等待开始
	StatusActive    Status = "active"    // 进行中
	StatusPaused    Status = "paused"    // 暂停
	StatusCompleted Status = "completed" // 已结束
)

// Room 会议室（管理多个会议）
type Room struct {
	mu       sync.RWMutex
	meetings map[string]*Meeting
	baseDir  string

	// 流式消息回调
	messageCallbacks []func(meetingID string, msg Message)
	typingCallbacks  map[string][]func(from string) // meetingID -> callbacks
}

// NewRoom 创建会议室
func NewRoom(baseDir string) *Room {
	return &Room{
		meetings:        make(map[string]*Meeting),
		baseDir:         baseDir,
		typingCallbacks: make(map[string][]func(from string)),
	}
}

// CreateMeeting 创建会议
func (r *Room) CreateMeeting(topic string, mode Mode, participants []string, createdBy string) (*Meeting, error) {
	id := generateID()

	meeting := &Meeting{
		ID:           id,
		Topic:        topic,
		Mode:         mode,
		Status:       StatusActive,
		Participants: participants,
		Messages:     make([]Message, 0),
		StartedAt:    time.Now(),
		CreatedBy:    createdBy,
	}

	// 添加系统消息
	meeting.AddMessage("system", MsgAction, fmt.Sprintf("会议 [%s] 开始，模式: %s", topic, mode))

	r.mu.Lock()
	r.meetings[id] = meeting
	r.mu.Unlock()

	// 确保目录存在
	meetingDir := filepath.Join(r.baseDir, id)
	os.MkdirAll(meetingDir, 0755)

	return meeting, nil
}

// GetMeeting 获取会议
func (r *Room) GetMeeting(id string) (*Meeting, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.meetings[id]
	return m, ok
}

// ListMeetings 列出会议
func (r *Room) ListMeetings() []*Meeting {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Meeting, 0, len(r.meetings))
	for _, m := range r.meetings {
		result = append(result, m)
	}
	return result
}

// AddMessage 添加消息（线程安全）
func (r *Room) AddMessage(meetingID string, from string, msgType MsgType, content string) (*Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	meeting, ok := r.meetings[meetingID]
	if !ok {
		return nil, fmt.Errorf("meeting not found: %s", meetingID)
	}

	if meeting.Status != StatusActive {
		return nil, fmt.Errorf("meeting is not active")
	}

	msg := Message{
		ID:        generateMsgID(),
		From:      from,
		Content:   content,
		Type:      msgType,
		Timestamp: time.Now(),
	}

	meeting.Messages = append(meeting.Messages, msg)

	// 触发回调
	for _, cb := range r.messageCallbacks {
		go cb(meetingID, msg)
	}

	return &msg, nil
}

// AddMentionMessage @某人
func (r *Room) AddMentionMessage(meetingID string, from string, content string, mentionTo []string) (*Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	meeting, ok := r.meetings[meetingID]
	if !ok {
		return nil, fmt.Errorf("meeting not found: %s", meetingID)
	}

	msg := Message{
		ID:        generateMsgID(),
		From:      from,
		Content:   content,
		Type:      MsgMention,
		MentionTo: mentionTo,
		Timestamp: time.Now(),
	}

	meeting.Messages = append(meeting.Messages, msg)

	for _, cb := range r.messageCallbacks {
		go cb(meetingID, msg)
	}

	return &msg, nil
}

// EndMeeting 结束会议
func (r *Room) EndMeeting(meetingID string, summary string, actionItems []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	meeting, ok := r.meetings[meetingID]
	if !ok {
		return fmt.Errorf("meeting not found: %s", meetingID)
	}

	now := time.Now()
	meeting.Status = StatusCompleted
	meeting.EndedAt = &now
	meeting.Summary = summary
	meeting.ActionItems = actionItems

	// 添加结束消息
	meeting.Messages = append(meeting.Messages, Message{
		ID:        generateMsgID(),
		From:      "system",
		Content:   fmt.Sprintf("会议结束。总结: %s", summary),
		Type:      MsgAction,
		Timestamp: now,
	})

	// 保存会议记录
	return r.saveMeeting(meeting)
}

// OnMessage 注册消息回调（流式推送）
func (r *Room) OnMessage(callback func(meetingID string, msg Message)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messageCallbacks = append(r.messageCallbacks, callback)
}

// BroadcastTyping 广播"正在输入"状态
func (r *Room) BroadcastTyping(meetingID string, from string) {
	r.mu.RLock()
	cbs := r.typingCallbacks[meetingID]
	r.mu.RUnlock()

	for _, cb := range cbs {
		go cb(from)
	}
}

// AddTypingCallback 注册输入状态回调
func (r *Room) AddTypingCallback(meetingID string, callback func(from string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.typingCallbacks[meetingID] = append(r.typingCallbacks[meetingID], callback)
}

// saveMeeting 保存会议记录到文件
func (r *Room) saveMeeting(m *Meeting) error {
	meetingDir := filepath.Join(r.baseDir, m.ID)
	os.MkdirAll(meetingDir, 0755)

	// 1. 保存 JSON（机器可读）
	jsonPath := filepath.Join(meetingDir, "meeting.json")
	jsonData, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		return err
	}

	// 2. 保存 Markdown（人类可读）
	mdPath := filepath.Join(meetingDir, "transcript.md")
	mdContent := r.formatMarkdown(m)
	if err := os.WriteFile(mdPath, []byte(mdContent), 0644); err != nil {
		return err
	}

	return nil
}

// formatMarkdown 生成 Markdown 格式会议记录
func (r *Room) formatMarkdown(m *Meeting) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s\n\n", m.Topic))
	sb.WriteString(fmt.Sprintf("- **会议ID**: %s\n", m.ID))
	sb.WriteString(fmt.Sprintf("- **模式**: %s\n", m.Mode))
	sb.WriteString(fmt.Sprintf("- **状态**: %s\n", m.Status))
	sb.WriteString(fmt.Sprintf("- **开始时间**: %s\n", m.StartedAt.Format("2006-01-02 15:04:05")))
	if m.EndedAt != nil {
		sb.WriteString(fmt.Sprintf("- **结束时间**: %s\n", m.EndedAt.Format("2006-01-02 15:04:05")))
	}
	sb.WriteString(fmt.Sprintf("- **参与者**: %s\n\n", strings.Join(m.Participants, ", ")))

	sb.WriteString("## 聊天记录\n\n")
	for _, msg := range m.Messages {
		timeStr := msg.Timestamp.Format("15:04:05")
		switch msg.Type {
		case MsgText:
			sb.WriteString(fmt.Sprintf("**[%s] %s**: %s\n\n", timeStr, msg.From, msg.Content))
		case MsgMention:
			sb.WriteString(fmt.Sprintf("**[%s] %s** @%s: %s\n\n",
				timeStr, msg.From, strings.Join(msg.MentionTo, ", "), msg.Content))
		case MsgAction:
			sb.WriteString(fmt.Sprintf("*[%s] %s*\n\n", timeStr, msg.Content))
		}
	}

	if m.Summary != "" {
		sb.WriteString("## 会议总结\n\n")
		sb.WriteString(m.Summary)
		sb.WriteString("\n\n")
	}

	if len(m.ActionItems) > 0 {
		sb.WriteString("## 行动项\n\n")
		for i, item := range m.ActionItems {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, item))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// GetTranscript 获取聊天历史
func (m *Meeting) GetTranscript() string {
	var parts []string
	for _, msg := range m.Messages {
		if msg.Type == MsgText || msg.Type == MsgMention {
			parts = append(parts, fmt.Sprintf("%s: %s", msg.From, msg.Content))
		}
	}
	return strings.Join(parts, "\n")
}

// AddMessage 内部方法：给 Meeting 添加消息
func (m *Meeting) AddMessage(from string, msgType MsgType, content string) {
	m.Messages = append(m.Messages, Message{
		ID:        generateMsgID(),
		From:      from,
		Content:   content,
		Type:      msgType,
		Timestamp: time.Now(),
	})
}

// 辅助函数
func generateID() string {
	return fmt.Sprintf("mtg-%d", time.Now().UnixNano())
}

func generateMsgID() string {
	return fmt.Sprintf("msg-%d", time.Now().UnixNano())
}
