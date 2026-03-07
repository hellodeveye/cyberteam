package session

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"cyberteam/internal/meeting"
	"cyberteam/internal/workflow"
)

// Session 当前会话状态
type Session struct {
	mu             sync.RWMutex
	currentProject *workflow.Project
	currentMeeting *meeting.Meeting
	privateChat    *PrivateChat // 当前私聊对象
}

// PrivateChat 私聊状态
type PrivateChat struct {
	With      string        // 对方名字
	StartedAt time.Time     // 开始时间
	History   []ChatMessage // 聊天记录
}

// ChatMessage 单条聊天消息
type ChatMessage struct {
	From      string
	Content   string
	Timestamp time.Time
}

// NewSession 创建新会话
func NewSession() *Session {
	return &Session{}
}

// SetProject 设置当前项目
func (s *Session) SetProject(p *workflow.Project) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentProject = p
}

// GetProject 获取当前项目
func (s *Session) GetProject() *workflow.Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentProject
}

// SetMeeting 设置当前会议
func (s *Session) SetMeeting(m *meeting.Meeting) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentMeeting = m
}

// GetMeeting 获取当前会议
func (s *Session) GetMeeting() *meeting.Meeting {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentMeeting
}

// InMeeting 检查是否在会议中
func (s *Session) InMeeting() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentMeeting != nil
}

// SetPrivateChat 设置私聊对象
func (s *Session) SetPrivateChat(with string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.privateChat = &PrivateChat{
		With:      with,
		StartedAt: time.Now(),
		History:   []ChatMessage{},
	}
}

// GetPrivateChat 获取当前私聊对象
func (s *Session) GetPrivateChat() *PrivateChat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.privateChat
}

// InPrivateChat 检查是否在私聊中
func (s *Session) InPrivateChat() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.privateChat != nil
}

// ExitPrivateChat 退出私聊
func (s *Session) ExitPrivateChat() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.privateChat = nil
}

// AddPrivateChatMessage 添加一条私聊消息到历史
func (s *Session) AddPrivateChatMessage(from, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.privateChat != nil {
		s.privateChat.History = append(s.privateChat.History, ChatMessage{
			From:      from,
			Content:   content,
			Timestamp: time.Now(),
		})
	}
}

// GetPrivateChatHistory 获取私聊历史记录（格式化为字符串）
func (s *Session) GetPrivateChatHistory() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.privateChat == nil || len(s.privateChat.History) == 0 {
		return ""
	}
	var lines []string
	for _, msg := range s.privateChat.History {
		lines = append(lines, fmt.Sprintf("%s: %s", msg.From, msg.Content))
	}
	return strings.Join(lines, "\n")
}

// GetPrompt 获取当前提示符
func (s *Session) GetPrompt() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// 私聊模式
	if s.privateChat != nil {
		return fmt.Sprintf("💬 [%s] > ", s.privateChat.With)
	}
	// 会议模式
	if s.currentMeeting != nil {
		return "🎤 > "
	}
	// 项目模式
	if s.currentProject != nil {
		return fmt.Sprintf("🎤 [%s] > ", s.currentProject.Name)
	}
	return "CyberTeam > "
}

// MessageQueue 异步消息队列
type MessageQueue struct {
	mu       sync.Mutex
	messages []string
	cond     *sync.Cond
}

// NewMessageQueue 创建消息队列
func NewMessageQueue() *MessageQueue {
	mq := &MessageQueue{}
	mq.cond = sync.NewCond(&mq.mu)
	return mq
}

// Push 添加消息
func (mq *MessageQueue) Push(msg string) {
	mq.mu.Lock()
	mq.messages = append(mq.messages, msg)
	mq.mu.Unlock()
	mq.cond.Signal()
}

// PopAll 取出所有消息
func (mq *MessageQueue) PopAll() []string {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	if len(mq.messages) == 0 {
		return nil
	}
	msgs := mq.messages
	mq.messages = nil
	return msgs
}

// Wait 等待消息
func (mq *MessageQueue) Wait() {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	if len(mq.messages) == 0 {
		mq.cond.Wait()
	}
}
