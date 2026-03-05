package protocol

import (
	"encoding/json"
	"fmt"
)

// MessageType 消息类型
type MessageType string

const (
	// 生命周期
	MsgRegister  MessageType = "register"  // Worker 注册
	MsgHeartbeat MessageType = "heartbeat" // 心跳
	MsgShutdown  MessageType = "shutdown"  // 优雅关闭

	// 任务相关
	MsgAssign   MessageType = "assign"   // Master 分配任务
	MsgAccept   MessageType = "accept"   // Worker 接受任务
	MsgReject   MessageType = "reject"   // Worker 拒绝任务
	MsgProgress MessageType = "progress" // 进度更新
	MsgComplete MessageType = "complete" // 任务完成
	MsgFailed   MessageType = "failed"   // 任务失败

	// 查询
	MsgQueryCap    MessageType = "query_capability" // 查询能力
	MsgListWorkers MessageType = "list_workers"     // 列出员工
)

// Message 基础消息结构
type Message struct {
	Type    MessageType    `json:"type"`
	ID      string         `json:"id"`                // 消息唯一ID
	TaskID  string         `json:"task_id,omitempty"` // 关联任务ID
	Payload map[string]any `json:"payload"`
}

// Capability 单个能力定义
type Capability struct {
	Name        string  `json:"name"`        // 能力名称
	Description string  `json:"description"` // 描述
	Inputs      []Param `json:"inputs"`      // 需要的输入
	Outputs     []Param `json:"outputs"`     // 产生的输出
	EstTime     string  `json:"est_time"`    // 预估耗时 (如 "5m", "2h")
}

type Param struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // string, number, bool, object, array
	Required bool   `json:"required"`
	Desc     string `json:"desc"`
}

// WorkerProfile 员工档案
type WorkerProfile struct {
	ID              string       `json:"id"`                         // 员工工号
	Name            string       `json:"name"`                       // 员工名字
	Role            string       `json:"role"`                       // 角色: product/tester/developer
	Version         string       `json:"version"`                    // 版本
	Capabilities    []Capability `json:"capabilities"`               // 技能列表
	Status          WorkerStatus `json:"status"`                     // 当前状态
	Load            int          `json:"load"`                       // 当前负载 (0-100)
	ProfileMarkdown string       `json:"profile_markdown,omitempty"` // PROFILE.md Markdown 正文
}

type WorkerStatus string

const (
	StatusIdle    WorkerStatus = "idle"    // 空闲
	StatusBusy    WorkerStatus = "busy"    // 忙
	StatusOffline WorkerStatus = "offline" // 离线
)

// Task 任务定义
type Task struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`               // 任务类型，匹配 Capability.Name
	Title        string         `json:"title"`              // 标题
	Description  string         `json:"description"`        // 详细描述
	Inputs       map[string]any `json:"inputs"`             // 输入参数
	Priority     int            `json:"priority"`           // 优先级 1-5
	Deadline     *int64         `json:"deadline,omitempty"` // 截止时间戳
	WorkspaceDir string         `json:"workspace_dir,omitempty"` // 工作空间目录
}

// TaskResult 任务结果
type TaskResult struct {
	TaskID   string         `json:"task_id"`
	Success  bool           `json:"success"`
	Outputs  map[string]any `json:"outputs"`
	Logs     []string       `json:"logs"`
	Error    string         `json:"error,omitempty"`
	Duration int64          `json:"duration_ms"` // 实际耗时
}

// Encode 编码消息为 JSON 行
func (m Message) Encode() ([]byte, error) {
	return json.Marshal(m)
}

// DecodeMessage 解码消息
func DecodeMessage(data []byte) (*Message, error) {
	var m Message
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("decode message: %w", err)
	}
	return &m, nil
}
