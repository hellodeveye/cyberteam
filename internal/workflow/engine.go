package workflow

import (
	"fmt"
	"sync"
	"time"
)

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

// Task 工作流任务
type Task struct {
	ID          string
	ProjectID   string
	Name        string
	Description string
	Stage       Stage
	Status      Status
	Assignee    string      // 分配给谁的ID
	Input       interface{} // 输入数据
	Output      interface{} // 输出结果
	Feedback    string      // 反馈/评审意见
	ParentID    string      // 父任务ID（用于子任务）
	WorkspaceDir string     // 工作空间目录（用于文件输出）

	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time

	// 工作流控制
	OnComplete func(*Task) // 完成回调
	OnReject   func(*Task) // 驳回回调
}

// Project 项目
type Project struct {
	ID          string
	Name        string
	Description string
	Status      Status

	CurrentStage Stage
	Tasks        map[Stage][]*Task // 各阶段的任务
	Artifacts    map[string]any    // 项目产出物
	WorkspaceDir string            // 工作空间目录

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Workflow 工作流定义
type Workflow struct {
	Stages []StageDefinition
}

// StageDefinition 阶段定义
type StageDefinition struct {
	Name        Stage
	Description string
	Assignable  []string                             // 可分配的角色
	NextStages  []Stage                              // 下一阶段（支持分支）
	OnComplete  func(*Engine, *Project, *Task) error // 阶段完成后的处理
}

// Engine 工作流引擎
type Engine struct {
	mu        sync.RWMutex
	projects  map[string]*Project
	tasks     map[string]*Task
	workflow  *Workflow
	workspace WorkspaceManager
	store     Storage

	// 事件监听
	handlers map[string][]func(interface{})
}

// WorkspaceManager 工作空间接口
type WorkspaceManager interface {
	CreateProjectWorkspace(projectID, projectName string) (string, error)
	WriteFile(projectName, projectID string, stageNum int, filename string, content []byte) error
	ReadFile(projectName, projectID string, stageNum int, filename string) ([]byte, error)
	GetProjectDir(projectName, projectID string) string
}

// Storage 存储接口
type Storage interface {
	SaveProject(project *Project) error
	LoadAllProjects() ([]*Project, error)
}

// NewEngine 创建工作流引擎
func NewEngine(wf *Workflow) *Engine {
	return &Engine{
		projects: make(map[string]*Project),
		tasks:    make(map[string]*Task),
		workflow: wf,
		handlers: make(map[string][]func(interface{})),
	}
}

// SetWorkspace 设置工作空间管理器
func (e *Engine) SetWorkspace(ws WorkspaceManager) {
	e.workspace = ws
}

// SetStorage 设置存储管理器
func (e *Engine) SetStorage(s Storage) {
	e.store = s
}

// LoadProjects 从存储加载所有项目
func (e *Engine) LoadProjects() error {
	if e.store == nil {
		return nil
	}

	projects, err := e.store.LoadAllProjects()
	if err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, project := range projects {
		e.projects[project.ID] = project
		// 重建任务索引
		for _, stageTasks := range project.Tasks {
			for _, task := range stageTasks {
				e.tasks[task.ID] = task
			}
		}
	}

	return nil
}

// CreateProject 创建新项目
func (e *Engine) CreateProject(name, description string) *Project {
	project := &Project{
		ID:           generateID(),
		Name:         name,
		Description:  description,
		Status:       StatusPending,
		CurrentStage: StageRequirement,
		Tasks:        make(map[Stage][]*Task),
		Artifacts:    make(map[string]any),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	e.mu.Lock()
	e.projects[project.ID] = project
	e.mu.Unlock()

	// 创建工作空间
	if e.workspace != nil {
		if wsDir, err := e.workspace.CreateProjectWorkspace(project.ID, project.Name); err == nil {
			project.WorkspaceDir = wsDir
		}
	}

	// 自动保存
	if e.store != nil {
		e.store.SaveProject(project)
	}

	e.emit("project.created", project)
	return project
}

// CreateTask 创建任务
func (e *Engine) CreateTask(projectID string, stage Stage, name, description string, input interface{}) *Task {
	e.mu.RLock()
	project, ok := e.projects[projectID]
	e.mu.RUnlock()

	task := &Task{
		ID:           generateID(),
		ProjectID:    projectID,
		Name:         name,
		Description:  description,
		Stage:        stage,
		Status:       StatusPending,
		Input:        input,
		WorkspaceDir: "", // 默认为空
		CreatedAt:    time.Now(),
	}

	// 如果项目存在，设置工作目录
	if ok && project != nil {
		task.WorkspaceDir = project.WorkspaceDir
	}

	e.mu.Lock()
	e.tasks[task.ID] = task
	if project != nil {
		project.Tasks[stage] = append(project.Tasks[stage], task)
	}
	e.mu.Unlock()

	e.emit("task.created", task)
	return task
}

// AssignTask 分配任务
func (e *Engine) AssignTask(taskID, assigneeID string) error {
	// 先获取任务，不持有锁
	e.mu.RLock()
	task, ok := e.tasks[taskID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	// 更新任务，使用写锁
	e.mu.Lock()
	task.Assignee = assigneeID
	task.Status = StatusAssigned
	now := time.Now()
	task.StartedAt = &now
	e.mu.Unlock()

	// 在锁外触发事件
	e.emit("task.assigned", task)
	return nil
}

// ClearTaskAssignee 清空任务分配人
func (e *Engine) ClearTaskAssignee(taskID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	task, ok := e.tasks[taskID]
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	task.Assignee = ""
	return nil
}

// CompleteTask 完成任务
func (e *Engine) CompleteTask(taskID string, output interface{}) error {
	e.mu.Lock()

	task, ok := e.tasks[taskID]
	if !ok {
		e.mu.Unlock()
		return fmt.Errorf("task not found: %s", taskID)
	}

	task.Status = StatusCompleted
	task.Output = output
	now := time.Now()
	task.CompletedAt = &now

	// 更新项目产出物
	if project, ok := e.projects[task.ProjectID]; ok {
		project.UpdatedAt = now
		// 保存阶段产出
		switch task.Stage {
		case StageRequirement:
			project.Artifacts["prd"] = output
		case StageDesign:
			project.Artifacts["design"] = output
		case StageDevelop:
			project.Artifacts["code"] = output
		case StageTest:
			project.Artifacts["test_report"] = output
		}
	}

	// 保存回调和任务，在锁外执行
	onComplete := task.OnComplete

	e.mu.Unlock()

	// 执行完成回调（锁外）
	if onComplete != nil {
		go onComplete(task)
	}

	e.emit("task.completed", task)

	// 自动推进工作流（锁外）
	go e.advanceWorkflow(taskID)

	return nil
}

// RejectTask 驳回任务（退回上一阶段）
func (e *Engine) RejectTask(taskID, feedback string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	task, ok := e.tasks[taskID]
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	task.Status = StatusRejected
	task.Feedback = feedback

	// 执行驳回回调
	if task.OnReject != nil {
		go task.OnReject(task)
	}

	e.emit("task.rejected", task)
	return nil
}

// advanceWorkflow 推进工作流到下一阶段
func (e *Engine) advanceWorkflow(taskID string) {
	e.mu.RLock()
	task, ok := e.tasks[taskID]
	if !ok {
		e.mu.RUnlock()
		return
	}
	stage := task.Stage
	projectID := task.ProjectID
	e.mu.RUnlock()

	// 查找阶段定义
	var currentDef *StageDefinition
	for i := range e.workflow.Stages {
		if e.workflow.Stages[i].Name == stage {
			currentDef = &e.workflow.Stages[i]
			break
		}
	}

	if currentDef == nil || currentDef.OnComplete == nil {
		return
	}

	e.mu.RLock()
	project := e.projects[projectID]
	e.mu.RUnlock()
	
	if project == nil {
		return
	}

	// 执行阶段完成处理
	if err := currentDef.OnComplete(e, project, task); err != nil {
		fmt.Printf("Workflow advance error: %v\n", err)
	}
}

// On 注册事件监听
func (e *Engine) On(event string, handler func(interface{})) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers[event] = append(e.handlers[event], handler)
}

// emit 触发事件
func (e *Engine) emit(event string, data interface{}) {
	e.mu.RLock()
	handlers := e.handlers[event]
	e.mu.RUnlock()

	for _, h := range handlers {
		go h(data)
	}
}

// GetProject 获取项目
func (e *Engine) GetProject(id string) *Project {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.projects[id]
}

// GetTask 获取任务
func (e *Engine) GetTask(id string) *Task {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.tasks[id]
}

// GetAllProjects 获取所有项目
func (e *Engine) GetAllProjects() []*Project {
	e.mu.RLock()
	defer e.mu.RUnlock()

	projects := make([]*Project, 0, len(e.projects))
	for _, p := range e.projects {
		projects = append(projects, p)
	}
	return projects
}

// GetAllTasks 获取所有任务
func (e *Engine) GetAllTasks() []*Task {
	e.mu.RLock()
	defer e.mu.RUnlock()

	tasks := make([]*Task, 0, len(e.tasks))
	for _, t := range e.tasks {
		tasks = append(tasks, t)
	}
	return tasks
}

var idCounter int64
var idMu sync.Mutex

func generateID() string {
	idMu.Lock()
	defer idMu.Unlock()
	idCounter++
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), idCounter)
}
