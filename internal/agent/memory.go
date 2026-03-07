package agent

import (
	"cyberteam/internal/llm"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Memory 接口 - 支持多种实现
type Memory interface {
	// AddMessage 添加消息到短期记忆
	AddMessage(role, content string)
	// AddToolResult 添加工具结果到短期记忆
	AddToolResult(toolName, result string)
	// GetMessages 获取所有消息 (短期 + 长期)
	GetMessages() []llm.Message
	// GetShortTerm 获取短期记忆
	GetShortTerm() []llm.Message
	// Clear 清空短期记忆
	Clear()
	// FlushToLongTerm 将短期记忆刷到长期记忆
	FlushToLongTerm()
	// Save 持久化到文件
	Save(path string) error
	// Load 从文件加载
	Load(path string) error
	// GetPersistentContent 获取持久化内容 (用于生成 prompt)
	GetPersistentContent() string
}

// MaxShortTermMessages 短期记忆最大消息数，超过时自动裁剪保留最近的消息
const MaxShortTermMessages = 50

// FileMemory 基于文件的记忆实现
// 支持读取 MEMORY.md 作为长期记忆
type FileMemory struct {
	mu           sync.RWMutex
	ShortTerm    []llm.Message // 当前会话
	LongTerm     []llm.Message // 从 MEMORY.md 加载的历史
	personalPath string        // 个人 MEMORY.md 路径
	sharedPath   string        // 共享 MEMORY.md 路径
}

// NewFileMemory 创建基于文件的记忆
func NewFileMemory() *FileMemory {
	return &FileMemory{
		ShortTerm: make([]llm.Message, 0),
		LongTerm:  make([]llm.Message, 0),
	}
}

// NewFileMemoryWithPaths 创建带路径的记忆
func NewFileMemoryWithPaths(personalPath, sharedPath string) *FileMemory {
	m := &FileMemory{
		ShortTerm:    make([]llm.Message, 0),
		LongTerm:     make([]llm.Message, 0),
		personalPath: personalPath,
		sharedPath:   sharedPath,
	}

	// 加载持久化记忆
	m.loadFromFiles()
	return m
}

// loadFromFiles 从文件加载记忆（调用者须持有锁或在初始化期间调用）
func (m *FileMemory) loadFromFiles() {
	// 清空长期记忆，避免重复追加
	m.LongTerm = make([]llm.Message, 0)

	// 加载个人记忆
	if m.personalPath != "" {
		if content, err := os.ReadFile(m.personalPath); err == nil {
			m.LongTerm = append(m.LongTerm, llm.Message{
				Role:    "system",
				Content: fmt.Sprintf("\n\n=== 个人记忆 (MEMORY.md) ===\n%s", string(content)),
			})
		}
	}

	// 加载共享记忆
	if m.sharedPath != "" {
		if content, err := os.ReadFile(m.sharedPath); err == nil {
			m.LongTerm = append(m.LongTerm, llm.Message{
				Role:    "system",
				Content: fmt.Sprintf("\n\n=== 团队共享知识 ===\n%s", string(content)),
			})
		}
	}
}

// AddMessage 添加消息到短期记忆（自动裁剪防止无限增长）
func (m *FileMemory) AddMessage(role, content string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ShortTerm = append(m.ShortTerm, llm.Message{
		Role:    role,
		Content: content,
	})
	// 超过上限时裁剪，保留最近的消息
	if len(m.ShortTerm) > MaxShortTermMessages {
		m.ShortTerm = m.ShortTerm[len(m.ShortTerm)-MaxShortTermMessages:]
	}
}

// AddToolResult 添加工具结果到短期记忆
func (m *FileMemory) AddToolResult(toolName, result string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ShortTerm = append(m.ShortTerm, llm.Message{
		Role:    "tool",
		Content: fmt.Sprintf("[%s] %s", toolName, result),
	})
	if len(m.ShortTerm) > MaxShortTermMessages {
		m.ShortTerm = m.ShortTerm[len(m.ShortTerm)-MaxShortTermMessages:]
	}
}

// GetMessages 获取所有消息
func (m *FileMemory) GetMessages() []llm.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]llm.Message, 0, len(m.LongTerm)+len(m.ShortTerm))
	result = append(result, m.LongTerm...)
	result = append(result, m.ShortTerm...)
	return result
}

// GetShortTerm 获取短期记忆
func (m *FileMemory) GetShortTerm() []llm.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ShortTerm
}

// Clear 清空短期记忆
func (m *FileMemory) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ShortTerm = nil
}

// FlushToLongTerm 将短期记忆刷到长期记忆
// 注意: FileMemory 不自动保存到文件，需要手动 Save
func (m *FileMemory) FlushToLongTerm() {
	// 对于文件实现，可以将短期记忆追加到长期记忆的末尾
	// 但不会自动写回 MEMORY.md
}

// Save 保存到文件 (只保存短期会话的持久化信息)
func (m *FileMemory) Save(path string) error {
	m.mu.RLock()
	// FileMemory 的 Save 主要保存会话元数据
	// MEMORY.md 由用户手动编辑，不自动覆盖
	data := struct {
		ShortTermCount int `json:"short_term_count"`
		LongTermCount  int `json:"long_term_count"`
	}{
		ShortTermCount: len(m.ShortTerm),
		LongTermCount:  len(m.LongTerm),
	}
	m.mu.RUnlock()

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal memory: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	if err := os.WriteFile(path, jsonData, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// Load 从文件加载
func (m *FileMemory) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read file: %w", err)
	}

	var parsed struct {
		ShortTermCount int `json:"short_term_count"`
		LongTermCount  int `json:"long_term_count"`
	}

	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("unmarshal memory: %w", err)
	}

	return nil
}

// GetPersistentContent 获取持久化内容 (从 MEMORY.md)
func (m *FileMemory) GetPersistentContent() string {
	var content string

	// 读取个人记忆
	if m.personalPath != "" {
		if data, err := os.ReadFile(m.personalPath); err == nil {
			content += string(data) + "\n\n"
		}
	}

	// 读取共享记忆
	if m.sharedPath != "" {
		if data, err := os.ReadFile(m.sharedPath); err == nil {
			content += string(data) + "\n"
		}
	}

	return content
}

// SetPersonalPath 设置个人记忆路径
func (m *FileMemory) SetPersonalPath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.personalPath = path
	m.loadFromFiles()
}

// SetSharedPath 设置共享记忆路径
func (m *FileMemory) SetSharedPath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sharedPath = path
	m.loadFromFiles()
}

// InMemoryMemory 纯内存实现 (用于测试)
type InMemoryMemory struct {
	ShortTerm []llm.Message
	LongTerm  []llm.Message
}

// NewInMemoryMemory 创建内存记忆
func NewInMemoryMemory() *InMemoryMemory {
	return &InMemoryMemory{
		ShortTerm: make([]llm.Message, 0),
		LongTerm:  make([]llm.Message, 0),
	}
}

func (m *InMemoryMemory) AddMessage(role, content string) {
	m.ShortTerm = append(m.ShortTerm, llm.Message{Role: role, Content: content})
}

func (m *InMemoryMemory) AddToolResult(toolName, result string) {
	m.ShortTerm = append(m.ShortTerm, llm.Message{
		Role:    "tool",
		Content: fmt.Sprintf("[%s] %s", toolName, result),
	})
}

func (m *InMemoryMemory) GetMessages() []llm.Message {
	var result []llm.Message
	result = append(result, m.LongTerm...)
	result = append(result, m.ShortTerm...)
	return result
}

func (m *InMemoryMemory) GetShortTerm() []llm.Message {
	return m.ShortTerm
}

func (m *InMemoryMemory) Clear() {
	m.ShortTerm = nil
}

func (m *InMemoryMemory) FlushToLongTerm() {
	m.LongTerm = append(m.LongTerm, m.ShortTerm...)
	m.ShortTerm = make([]llm.Message, 0)
}

func (m *InMemoryMemory) Save(path string) error {
	data := struct {
		LongTerm []llm.Message `json:"long_term"`
	}{
		LongTerm: m.LongTerm,
	}
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal memory: %w", err)
	}
	return os.WriteFile(path, jsonData, 0644)
}

func (m *InMemoryMemory) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var parsed struct {
		LongTerm []llm.Message `json:"long_term"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	m.LongTerm = parsed.LongTerm
	return nil
}

func (m *InMemoryMemory) GetPersistentContent() string {
	return ""
}
