package main

import (
	"encoding/json"
	"fmt"
	"time"

	"cyberteam/internal/common"
	"cyberteam/internal/master"
	"cyberteam/internal/session"
	"cyberteam/internal/workflow"
	"cyberteam/internal/workspace"
)

// setupEventListeners 设置事件监听
func setupEventListeners(engine *workflow.Engine, mq *session.MessageQueue, sess *session.Session, wsManager *workspace.Manager, boss *master.Manager) {
	engine.On("project.created", func(data interface{}) {
		project := data.(*workflow.Project)
		mq.Push(fmt.Sprintf("🎉 新项目启动: %s", project.Name))
		if project.WorkspaceDir != "" {
			mq.Push(fmt.Sprintf("   📁 工作空间: %s", project.WorkspaceDir))
		}
	})

	engine.On("task.created", func(data interface{}) {
		task := data.(*workflow.Task)
		mq.Push(fmt.Sprintf("📋 新任务: [%s] %s", common.GetStageName(task.Stage), task.Name))

		// 自动分配任务
		if boss != nil && task.Assignee == "" {
			go func(t *workflow.Task) {
				time.Sleep(500 * time.Millisecond) // 稍等确保 Staff 已注册
				if err := boss.AssignWorkflowTask(t.ID); err != nil {
					mq.Push(fmt.Sprintf("   ⚠️ 自动分配失败: %v", err))
				}
			}(task)
		}
	})

	engine.On("task.assigned", func(data interface{}) {
		task := data.(*workflow.Task)
		mq.Push(fmt.Sprintf("👤 任务分配: %s → %s", task.Name, task.Assignee))
	})

	engine.On("task.completed", func(data interface{}) {
		task := data.(*workflow.Task)
		mq.Push(fmt.Sprintf("✅ 任务完成: %s [%s]", task.Name, common.GetStageName(task.Stage)))

		// 保存产物到工作空间
		if task.Output != nil && wsManager != nil {
			project := engine.GetProject(task.ProjectID)
			if project != nil && project.WorkspaceDir != "" {
				stageNum := common.GetStageNumber(task.Stage)
				filename := fmt.Sprintf("%s-output.json", task.Stage)

				// 将输出转为 JSON
				content, err := json.MarshalIndent(task.Output, "", "  ")
				if err == nil {
					err = wsManager.WriteFile(project.Name, project.ID, stageNum, filename, content)
					if err == nil {
						mq.Push(fmt.Sprintf("   💾 已保存到: %s/%s", common.GetStageDirName(stageNum), filename))
					}
				}
			}
		}

		// 显示产出物提示
		if task.Output != nil {
			switch task.Stage {
			case workflow.StageRequirement:
				mq.Push("   📄 PRD 已生成，用 'artifacts' 或 'show prd' 查看")
			case workflow.StageDevelop:
				mq.Push("   💻 代码已生成，用 'show code' 查看")
			case workflow.StageTest:
				mq.Push("   🧪 测试报告已生成，用 'show test_report' 查看")
			}
		}
		mq.Push(fmt.Sprintf("💡 用 'approve %s' 继续，或 'reject %s <原因>' 打回", task.ID[:8], task.ID[:8]))
	})

	engine.On("task.rejected", func(data interface{}) {
		task := data.(*workflow.Task)
		mq.Push(fmt.Sprintf("🔄 任务被驳回: %s", task.Name))
		if task.Feedback != "" {
			mq.Push(fmt.Sprintf("   反馈: %s", task.Feedback))
		}
	})
}
