package workflow

import "fmt"

// CreateDevWorkflow 创建标准软件开发工作流
// 需求 → 设计 → 评审 → 开发 → 测试 → 部署
func CreateDevWorkflow() *Workflow {
	return &Workflow{
		Stages: []StageDefinition{
			{
				Name:        StageRequirement,
				Description: "产品需求分析",
				Assignable:  []string{"product"},
				NextStages:  []Stage{StageDesign},
				OnComplete: func(engine *Engine, project *Project, task *Task) error {
					// PRD 完成后，自动创建设计任务
					// 从 task.Output 中提取 PRD 内容（支持字符串或嵌套对象）
					var prdContent string
					if output, ok := task.Output.(map[string]any); ok {
						if prd, ok := output["prd"].(string); ok && prd != "" {
							prdContent = prd
						} else {
							// 如果没有 prd 字段，使用整个输出作为内容
							prdContent = fmt.Sprintf("%v", output)
						}
					} else if str, ok := task.Output.(string); ok {
						prdContent = str
					}

					designTask := engine.CreateTask(project.ID, StageDesign,
						fmt.Sprintf("设计: %s", project.Name),
						"基于PRD进行系统设计",
						map[string]any{
							"prd":      prdContent,
							"feedback": "",
						})

					// 设计完成后需要评审（任务级回调）
					designTask.OnComplete = func(t *Task) {
						// 从设计任务输出中提取设计内容
						var designContent string
						if output, ok := t.Output.(map[string]any); ok {
							if design, ok := output["design"].(string); ok && design != "" {
								designContent = design
							} else {
								designContent = fmt.Sprintf("%v", output)
							}
						} else if str, ok := t.Output.(string); ok {
							designContent = str
						}

						engine.CreateTask(project.ID, StageReview,
							fmt.Sprintf("评审设计: %s", project.Name),
							"评审系统设计方案",
							map[string]any{
								"design": designContent,
								"prd":    prdContent,
							})
					}
					return nil
				},
			},
			{
				Name:        StageDesign,
				Description: "系统设计",
				Assignable:  []string{"product", "developer"},
				NextStages:  []Stage{StageReview},
				// 注意：评审任务在 StageRequirement 的 designTask.OnComplete 中创建
				// 这里不需要 OnComplete，避免重复创建
				OnComplete: nil,
			},
			{
				Name:        StageReview,
				Description: "设计评审",
				Assignable:  []string{"product", "developer", "tester"},
				NextStages:  []Stage{StageDevelop, StageDesign}, // 通过或打回
				OnComplete: func(engine *Engine, project *Project, task *Task) error {
					// 评审通过后创建开发任务
					// 从评审任务输入中获取设计内容（评审任务不改变设计内容）
					var designContent string
					if input, ok := task.Input.(map[string]any); ok {
						if design, ok := input["design"].(string); ok {
							designContent = design
						}
					}

					devTask := engine.CreateTask(project.ID, StageDevelop,
						fmt.Sprintf("开发: %s", project.Name),
						"根据设计文档进行开发",
						map[string]any{
							"design": designContent,
							"prd":    project.Artifacts["prd"],
						})

					// 开发完成后创建测试任务
					devTask.OnComplete = func(t *Task) {
						testTask := engine.CreateTask(project.ID, StageTest,
							fmt.Sprintf("测试: %s", project.Name),
							"功能测试",
							map[string]any{
								"code":   t.Output,
								"design": project.Artifacts["design"],
							})

						// 测试完成后，如果发现问题打回开发
						testTask.OnComplete = func(tt *Task) {
							result := tt.Output.(map[string]any)
							if bugs, ok := result["bugs"].([]interface{}); ok && len(bugs) > 0 {
								// 有bug，创建修复任务
								fixTask := engine.CreateTask(project.ID, StageDevelop,
									fmt.Sprintf("修复Bug: %s", project.Name),
									"修复测试发现的问题",
									map[string]any{
										"bugs": bugs,
										"code": project.Artifacts["code"],
									})
								fixTask.OnComplete = func(ft *Task) {
									// 修复后重新测试
									engine.CreateTask(project.ID, StageTest,
										fmt.Sprintf("回归测试: %s", project.Name),
										"验证Bug修复",
										map[string]any{
											"code":   ft.Output,
											"design": project.Artifacts["design"],
										})
								}
							}
						}
					}
					return nil
				},
			},
			{
				Name:        StageDevelop,
				Description: "功能开发",
				Assignable:  []string{"developer"},
				NextStages:  []Stage{StageTest, StageDesign},
				// 注意：测试任务在 StageReview 的 devTask.OnComplete 中创建
				OnComplete: nil,
			},
			{
				Name:        StageTest,
				Description: "功能测试",
				Assignable:  []string{"tester"},
				NextStages:  []Stage{StageDeploy, StageDevelop}, // 通过或打回修复
				// 注意：后续任务在 testTask.OnComplete 中处理
				OnComplete: nil,
			},
			{
				Name:        StageDeploy,
				Description: "部署上线",
				Assignable:  []string{"developer"},
				NextStages:  []Stage{StageDone},
				OnComplete: func(engine *Engine, project *Project, task *Task) error {
					// 部署完成后，项目结束
					project.Status = StatusCompleted
					return nil
				},
			},
		},
	}
}
