package protocol

import "time"

// Stage 工作流阶段
type Stage string

const (
	StageRequirement Stage = "requirement" // 需求阶段
	StageDesign      Stage = "design"      // 设计阶段
	StageReview      Stage = "review"      // 评审阶段
	StageDevelop     Stage = "develop"     // 开发阶段
	StageTest        Stage = "test"        // 测试阶段
	StageDeploy      Stage = "deploy"      // 部署阶段
	StageDone        Stage = "done"        // 完成
)

// Status 任务状态
type Status string

const (
	StatusPending    Status = "pending"     // 待处理
	StatusAssigned   Status = "assigned"    // 已分配
	StatusInProgress Status = "in_progress" // 进行中
	StatusCompleted  Status = "completed"   // 已完成
	StatusRejected   Status = "rejected"    // 被驳回（需要重做）
	StatusFailed     Status = "failed"      // 失败
)

// WorkflowTask 工作流任务（区别于 message.go 中的 Task，用于通信协议）
type WorkflowTask struct {
	ID           string
	ProjectID    string
	Name         string
	Description  string
	Stage        Stage
	Status       Status
	Assignee     string      // 分配给谁的ID
	Input        interface{} // 输入数据
	Output       interface{} // 输出结果
	Feedback     string      // 反馈/评审意见
	ParentID     string      // 父任务ID（用于子任务）
	WorkspaceDir string      // 工作空间目录（用于文件输出）

	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time

	// 工作流控制（不持久化）
	OnComplete func(*WorkflowTask) `json:"-"` // 完成回调
	OnReject   func(*WorkflowTask) `json:"-"` // 驳回回调
}

// Project 项目
type Project struct {
	ID          string
	Name        string
	Description string
	Status      Status

	CurrentStage Stage
	Tasks        map[Stage][]*WorkflowTask // 各阶段的任务
	Artifacts    map[string]any            // 项目产出物
	WorkspaceDir string                    // 工作空间目录

	CreatedAt time.Time
	UpdatedAt time.Time
}
