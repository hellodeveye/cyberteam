package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"cyber-company/internal/workflow"
)

// ProjectData 项目持久化数据
type ProjectData struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Status       workflow.Status        `json:"status"`
	CurrentStage workflow.Stage         `json:"current_stage"`
	WorkspaceDir string                 `json:"workspace_dir"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	Tasks        []TaskData             `json:"tasks"`
	Artifacts    map[string]interface{} `json:"artifacts"`
}

// TaskData 任务持久化数据
type TaskData struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Stage       workflow.Stage  `json:"stage"`
	Status      workflow.Status `json:"status"`
	Assignee    string          `json:"assignee"`
	Input       interface{}     `json:"input"`
	Output      interface{}     `json:"output"`
	Feedback    string          `json:"feedback"`
	ParentID    string          `json:"parent_id"`
	CreatedAt   time.Time       `json:"created_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

// Store 存储管理器
type Store struct {
	baseDir string
}

// NewStore 创建存储管理器
func NewStore(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

// SaveProject 保存项目到磁盘
func (s *Store) SaveProject(project *workflow.Project) error {
	if project.WorkspaceDir == "" {
		return fmt.Errorf("project has no workspace dir")
	}

	data := ProjectData{
		ID:           project.ID,
		Name:         project.Name,
		Description:  project.Description,
		Status:       project.Status,
		CurrentStage: project.CurrentStage,
		WorkspaceDir: project.WorkspaceDir,
		CreatedAt:    project.CreatedAt,
		UpdatedAt:    time.Now(),
		Artifacts:    project.Artifacts,
	}

	// 收集所有任务
	for _, stageTasks := range project.Tasks {
		for _, task := range stageTasks {
			taskData := TaskData{
				ID:          task.ID,
				ProjectID:   task.ProjectID,
				Name:        task.Name,
				Description: task.Description,
				Stage:       task.Stage,
				Status:      task.Status,
				Assignee:    task.Assignee,
				Input:       task.Input,
				Output:      task.Output,
				Feedback:    task.Feedback,
				ParentID:    task.ParentID,
				CreatedAt:   task.CreatedAt,
				CompletedAt: task.CompletedAt,
			}
			data.Tasks = append(data.Tasks, taskData)
		}
	}

	// 写入文件
	filepath := filepath.Join(project.WorkspaceDir, "project.json")
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal project: %w", err)
	}

	return os.WriteFile(filepath, jsonData, 0644)
}

// LoadAllProjects 从工作空间加载所有项目
func (s *Store) LoadAllProjects() ([]*workflow.Project, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var projects []*workflow.Project
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectDir := filepath.Join(s.baseDir, entry.Name())
		project, err := s.LoadProject(projectDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load project from %s: %v\n", projectDir, err)
			continue
		}
		if project != nil {
			projects = append(projects, project)
		}
	}

	return projects, nil
}

// LoadProject 从目录加载单个项目
func (s *Store) LoadProject(projectDir string) (*workflow.Project, error) {
	filepath := filepath.Join(projectDir, "project.json")

	data, err := os.ReadFile(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var projectData ProjectData
	if err := json.Unmarshal(data, &projectData); err != nil {
		return nil, fmt.Errorf("unmarshal project: %w", err)
	}

	// 重建项目
	project := &workflow.Project{
		ID:           projectData.ID,
		Name:         projectData.Name,
		Description:  projectData.Description,
		Status:       projectData.Status,
		CurrentStage: projectData.CurrentStage,
		WorkspaceDir: projectData.WorkspaceDir,
		CreatedAt:    projectData.CreatedAt,
		UpdatedAt:    projectData.UpdatedAt,
		Tasks:        make(map[workflow.Stage][]*workflow.Task),
		Artifacts:    projectData.Artifacts,
	}

	// 重建任务
	for _, taskData := range projectData.Tasks {
		task := &workflow.Task{
			ID:          taskData.ID,
			ProjectID:   taskData.ProjectID,
			Name:        taskData.Name,
			Description: taskData.Description,
			Stage:       taskData.Stage,
			Status:      taskData.Status,
			Assignee:    taskData.Assignee,
			Input:       taskData.Input,
			Output:      taskData.Output,
			Feedback:    taskData.Feedback,
			ParentID:    taskData.ParentID,
			CreatedAt:   taskData.CreatedAt,
			CompletedAt: taskData.CompletedAt,
		}
		project.Tasks[task.Stage] = append(project.Tasks[task.Stage], task)
	}

	return project, nil
}

// AutoSave 自动保存钩子
func (s *Store) AutoSave(engine *workflow.Engine) {
	// 监听事件自动保存
	engine.On("project.created", func(data interface{}) {
		project := data.(*workflow.Project)
		s.SaveProject(project)
	})

	engine.On("task.created", func(data interface{}) {
		task := data.(*workflow.Task)
		project := engine.GetProject(task.ProjectID)
		if project != nil {
			s.SaveProject(project)
		}
	})

	engine.On("task.assigned", func(data interface{}) {
		task := data.(*workflow.Task)
		project := engine.GetProject(task.ProjectID)
		if project != nil {
			s.SaveProject(project)
		}
	})

	engine.On("task.completed", func(data interface{}) {
		task := data.(*workflow.Task)
		project := engine.GetProject(task.ProjectID)
		if project != nil {
			s.SaveProject(project)
		}
	})

	engine.On("task.rejected", func(data interface{}) {
		task := data.(*workflow.Task)
		project := engine.GetProject(task.ProjectID)
		if project != nil {
			s.SaveProject(project)
		}
	})
}
