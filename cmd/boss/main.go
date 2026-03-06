package main

import (
	"cyber-company/internal/master"
	"cyber-company/internal/storage"
	"cyber-company/internal/workflow"
	"cyber-company/internal/workspace"
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 全局变量（简化命令处理）
var gWsManager *workspace.Manager
var gBoss *master.Manager

// Session 当前会话状态
type Session struct {
	mu             sync.RWMutex
	currentProject *workflow.Project
}

func NewSession() *Session {
	return &Session{}
}

func (s *Session) SetProject(p *workflow.Project) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentProject = p
}

func (s *Session) GetProject() *workflow.Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentProject
}

func (s *Session) GetPrompt() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.currentProject == nil {
		return "🎤 > "
	}
	return fmt.Sprintf("🎤 [%s] > ", s.currentProject.Name)
}

// MessageQueue 异步消息队列
type MessageQueue struct {
	mu       sync.Mutex
	messages []string
	cond     *sync.Cond
}

func NewMessageQueue() *MessageQueue {
	mq := &MessageQueue{}
	mq.cond = sync.NewCond(&mq.mu)
	return mq
}

func (mq *MessageQueue) Push(msg string) {
	mq.mu.Lock()
	mq.messages = append(mq.messages, msg)
	mq.mu.Unlock()
	mq.cond.Signal()
}

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

func (mq *MessageQueue) Wait() {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	if len(mq.messages) == 0 {
		mq.cond.Wait()
	}
}

func main() {
	fmt.Println("🏢 AI Agent 软件公司")
	fmt.Println("====================")

	// 获取项目路径
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	rootDir := filepath.Join(exeDir, "..")
	if filepath.Base(exeDir) == "boss" {
		rootDir = filepath.Join(exeDir, "../..")
	}
	rootDir, _ = filepath.Abs(rootDir)

	// 创建工作流引擎
	wf := workflow.CreateDevWorkflow()
	engine := workflow.NewEngine(wf)

	// 创建工作空间管理器
	workspaceDir := filepath.Join(rootDir, "workspaces")
	wsManager := workspace.NewManager(workspaceDir)
	engine.SetWorkspace(wsManager)
	gWsManager = wsManager // 设置全局变量

	// 创建存储管理器
	store := storage.NewStore(workspaceDir)
	engine.SetStorage(store)

	// 加载已有项目
	if err := engine.LoadProjects(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load projects: %v\n", err)
	} else {
		projects := engine.GetAllProjects()
		if len(projects) > 0 {
			fmt.Printf("📂 已加载 %d 个历史项目\n", len(projects))
			for _, p := range projects {
				fmt.Printf("   - %s (%s)\n", p.Name, p.Status)
			}
			fmt.Println()
		}
	}

	// 设置自动保存
	store.AutoSave(engine)

	// 消息队列用于异步显示
	msgQueue := NewMessageQueue()

	// 会话状态
	session := NewSession()

	// 设置事件监听（推送到队列）
	// 注意：需要在 boss 创建后再调用 setupEventListeners，并传入 boss

	// 创建 Boss（项目经理）
	boss := master.NewManager(engine)
	gBoss = boss // 设置全局变量

	// 设置 Staff 消息回调
	boss.SetMessageCallback(func(staffID, msgType, content string) {
		msgQueue.Push(fmt.Sprintf("[%s] %s", staffID[:8], content))
	})

	// 招聘员工
	staffs := []struct {
		role   string
		name   string
		binary string
	}{
		{"product", "张产品", filepath.Join(rootDir, "cmd/staff/product/product")},
		{"developer", "李开发", filepath.Join(rootDir, "cmd/staff/developer/developer")},
		{"tester", "王测试", filepath.Join(rootDir, "cmd/staff/tester/tester")},
	}

	fmt.Println("\n📋 正在组建团队...")
	for _, s := range staffs {
		if _, err := boss.HireStaff(s.role, s.name, s.binary); err != nil {
			fmt.Printf("❌ %s 入职失败: %v\n", s.name, err)
		} else {
			time.Sleep(200 * time.Millisecond)
		}
	}

	fmt.Println("\n✅ 团队组建完成！")
	time.Sleep(500 * time.Millisecond)

	// 设置事件监听（需要在 boss 创建后）
	setupEventListeners(engine, msgQueue, session, wsManager, boss)

	// 恢复未完成的任务
	resumeTasks(engine, boss)

	// 显示帮助
	printHelp()

	// 启动消息显示 goroutine
	go func() {
		for {
			msgQueue.Wait()
			msgs := msgQueue.PopAll()
			for _, msg := range msgs {
				fmt.Printf("\r%-80s\n", "")
				fmt.Println(msg)
				fmt.Print(session.GetPrompt())
			}
		}
	}()

	// 交互式命令行
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print(session.GetPrompt())

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			fmt.Print(session.GetPrompt())
			continue
		}

		parts := strings.SplitN(line, " ", 3)
		cmd := parts[0]

		switch cmd {
		case "new":
			handleNew(engine, boss, session, parts)
		case "projects", "ls":
			handleProjects(engine, session)
		case "project", "cd":
			handleProject(engine, session, parts)
		case "..":
			handleExitProject(session)
		case "status", "st":
			handleStatus(engine, session)
		case "tasks":
			handleTasks(engine, session)
		case "watch":
			handleWatch(engine, session, parts)
		case "artifacts", "art":
			handleArtifacts(engine, session, gWsManager)
		case "show", "cat":
			handleShow(engine, session, parts)
		case "approve", "ok":
			handleApprove(engine, session, parts)
		case "reject", "no":
			handleReject(engine, session, parts)
		case "team":
			boss.ShowTeam()
		case "help", "h":
			printHelp()
		case "exit", "quit":
			fmt.Println("\n👋 正在关闭公司...")
			boss.Shutdown()
			fmt.Println("再见！")
			return
		default:
			fmt.Println("未知命令，输入 'help' 查看帮助")
		}

		fmt.Print(session.GetPrompt())
	}
}

func printHelp() {
	fmt.Println("\n📋 可用命令:")
	fmt.Println("  new <项目名> <描述>    创建新项目")
	fmt.Println("  projects, ls           列出所有项目")
	fmt.Println("  project <ID>, cd <ID>  进入项目（类似 tmux 切换会话）")
	fmt.Println("  ..                     退出当前项目")
	fmt.Println("  status, st             查看当前项目状态")
	fmt.Println("  tasks                  查看任务列表")
	fmt.Println("  watch <ID> [选项]      观察任务执行日志")
	fmt.Println("                         -f  实时跟踪")
	fmt.Println("                         -n  指定显示条数（默认50）")
	fmt.Println("  artifacts, art         查看产出物列表")
	fmt.Println("  show <name>, cat       查看产出物内容 (如: show prd)")
	fmt.Println("  approve <ID>, ok <ID>  批准任务")
	fmt.Println("  reject <ID> <原因>     驳回任务")
	fmt.Println("  team                   查看团队状态")
	fmt.Println("  help                   显示帮助")
	fmt.Println("  exit                   退出")
	fmt.Println()
	fmt.Println("💡 提示: 先用 'project <ID>' 进入项目，然后直接操作")
	fmt.Println()
}

func handleNew(engine *workflow.Engine, boss *master.Manager, session *Session, parts []string) {
	if len(parts) < 2 {
		fmt.Println("用法: new <项目名> [描述]")
		return
	}
	name := parts[1]
	desc := ""
	if len(parts) > 2 {
		desc = parts[2]
	}
	project := engine.CreateProject(name, desc)
	fmt.Printf("✅ 项目创建: %s (ID: %s)\n", name, project.ID[:8])

	// 自动创建需求分析任务
	task := engine.CreateTask(project.ID, workflow.StageRequirement,
		"需求分析: "+name,
		"分析需求并输出PRD文档",
		map[string]any{
			"requirement": desc,
			"constraints": "",
		})

	// 分配给产品经理
	if err := boss.AssignWorkflowTask(task.ID); err != nil {
		fmt.Printf("❌ 任务分配失败: %v\n", err)
	} else {
		fmt.Printf("📋 已创建需求分析任务\n")
		// 自动进入项目
		session.SetProject(project)
		fmt.Printf("\n🔀 已进入项目 [%s]\n", name)
	}
}

func handleProjects(engine *workflow.Engine, session *Session) {
	projects := engine.GetAllProjects()
	if len(projects) == 0 {
		fmt.Println("📭 暂无项目")
		return
	}

	current := session.GetProject()

	fmt.Println("\n📊 项目列表:")
	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("%-4s %-12s %-20s %-12s %-15s\n", "", "ID", "名称", "状态", "当前阶段")
	fmt.Println(strings.Repeat("-", 70))

	for _, p := range projects {
		marker := ""
		if current != nil && p.ID == current.ID {
			marker = "👉"
		}
		fmt.Printf("%-4s %-12s %-20s %-12s %-15s\n",
			marker,
			p.ID[:8],
			truncate(p.Name, 18),
			p.Status,
			getStageName(p.CurrentStage))
	}
	fmt.Println()
	fmt.Println("💡 用 'project <ID>' 或 'cd <ID>' 进入项目")
}

func handleProject(engine *workflow.Engine, session *Session, parts []string) {
	if len(parts) < 2 {
		fmt.Println("用法: project <项目ID>")
		return
	}
	inputID := parts[1]

	// 查找项目
	project := engine.GetProject(inputID)
	if project == nil {
		// 前缀匹配
		projects := engine.GetAllProjects()
		for _, p := range projects {
			if strings.HasPrefix(p.ID, inputID) {
				if project != nil {
					fmt.Println("❌ 多个项目匹配该前缀")
					return
				}
				project = p
			}
		}
	}

	if project == nil {
		fmt.Println("❌ 项目不存在")
		return
	}

	session.SetProject(project)
	fmt.Printf("\n🔀 已进入项目 [%s]\n", project.Name)
	showProjectStatus(project)
}

func handleExitProject(session *Session) {
	session.SetProject(nil)
	fmt.Println("📤 已退出项目")
}

func handleStatus(engine *workflow.Engine, session *Session) {
	project := session.GetProject()
	if project == nil {
		fmt.Println("❌ 未选择项目，先用 'project <ID>' 进入")
		return
	}
	showProjectStatus(project)
}

func showProjectStatus(project *workflow.Project) {
	fmt.Printf("\n📁 项目: %s\n", project.Name)
	fmt.Printf("   ID: %s\n", project.ID)
	fmt.Printf("   描述: %s\n", project.Description)
	fmt.Printf("   状态: %s\n", project.Status)
	fmt.Printf("   当前阶段: %s\n", getStageName(project.CurrentStage))
	fmt.Printf("   创建时间: %s\n", project.CreatedAt.Format("2006-01-02 15:04"))

	// 显示各阶段最新任务
	fmt.Println("\n📋 最新任务:")
	stages := []workflow.Stage{
		workflow.StageRequirement,
		workflow.StageDesign,
		workflow.StageReview,
		workflow.StageDevelop,
		workflow.StageTest,
		workflow.StageDeploy,
	}

	for _, stage := range stages {
		tasks := project.Tasks[stage]
		if len(tasks) == 0 {
			continue
		}
		// 显示最新任务
		task := tasks[len(tasks)-1]
		assignee := "未分配"
		if task.Assignee != "" {
			assignee = strings.Split(task.Assignee, "-")[0]
		}
		statusIcon := getStatusIcon(task.Status)
		fmt.Printf("   [%s] %s %s (%s)\n", getStageName(stage), statusIcon, task.Name, assignee)
	}

	// 显示产出物
	if len(project.Artifacts) > 0 {
		fmt.Println("\n📦 产出物:")
		for key := range project.Artifacts {
			fmt.Printf("   - %s (用 'show %s' 查看)\n", key, key)
		}
	}
	fmt.Println()
}

func handleTasks(engine *workflow.Engine, session *Session) {
	project := session.GetProject()

	var tasks []*workflow.Task
	if project != nil {
		// 显示当前项目的任务
		for _, stageTasks := range project.Tasks {
			tasks = append(tasks, stageTasks...)
		}
	} else {
		// 显示所有任务
		tasks = engine.GetAllTasks()
	}

	if len(tasks) == 0 {
		fmt.Println("📭 暂无任务")
		return
	}

	fmt.Println("\n📋 任务列表:")
	fmt.Println(strings.Repeat("-", 85))
	fmt.Printf("%-14s %-20s %-12s %-12s %-10s\n", "ID", "名称", "阶段", "状态", "负责人")
	fmt.Println(strings.Repeat("-", 85))

	for _, t := range tasks {
		assignee := t.Assignee
		if assignee == "" {
			assignee = "-"
		} else {
			parts := strings.Split(assignee, "-")
			if len(parts) > 0 {
				assignee = parts[0]
			}
		}
		// 显示完整 ID 或足够区分的前缀
		displayID := t.ID
		if len(displayID) > 12 {
			displayID = displayID[:12]
		}
		fmt.Printf("%-14s %-20s %-12s %-12s %-10s\n",
			displayID,
			truncate(t.Name, 18),
			getStageName(t.Stage),
			t.Status,
			assignee)
	}
	fmt.Println()
	if project != nil {
		fmt.Println("💡 用 'approve <ID>' 批准任务，'reject <ID> <原因>' 驳回")
	}
}

func handleWatch(engine *workflow.Engine, session *Session, parts []string) {
	if len(parts) < 2 {
		fmt.Println("用法: watch <任务ID> [选项]")
		fmt.Println("选项:")
		fmt.Println("  -f, --follow  实时跟踪（类似 tail -f）")
		fmt.Println("  -n <num>      显示最近 n 条日志（默认 50）")
		fmt.Println("示例:")
		fmt.Println("  watch 17727282        # 显示最近 50 条日志")
		fmt.Println("  watch 17727282 -f     # 实时跟踪")
		fmt.Println("  watch 17727282 -n 100 # 显示最近 100 条")
		return
	}

	inputID := parts[1]
	taskID := resolveTaskID(engine, inputID, session)
	if taskID == "" {
		fmt.Println("❌ 任务不存在（或多个匹配）")
		return
	}

	task := engine.GetTask(taskID)
	if task == nil {
		fmt.Println("❌ 任务不存在")
		return
	}

	// 解析选项
	follow := false
	limit := 50

	for i := 2; i < len(parts); i++ {
		switch parts[i] {
		case "-f", "--follow":
			follow = true
		case "-n":
			if i+1 < len(parts) {
				if n, err := strconv.Atoi(parts[i+1]); err == nil && n > 0 {
					limit = n
					i++
				}
			}
		}
	}

	// 显示任务信息
	fmt.Printf("\n👀 观察任务: %s [%s]\n", task.Name, getStageName(task.Stage))
	fmt.Printf("   状态: %s\n", task.Status)
	fmt.Printf("   负责人: %s\n", task.Assignee)
	fmt.Println(strings.Repeat("-", 60))

	// 如果是实时跟踪模式
	if follow {
		fmt.Println("🔄 实时跟踪模式（按 Ctrl+C 退出）...")
		fmt.Println()

		lastCount := 0
		for {
			logs := gBoss.GetTaskLogs(taskID, 0) // 获取所有日志

			// 只显示新日志
			if len(logs) > lastCount {
				newLogs := logs[lastCount:]
				for _, log := range newLogs {
					timeStr := log.Time.Format("15:04:05")
					icon := "📝"
					if log.Level == "error" {
						icon = "❌"
					} else if log.Level == "success" {
						icon = "✅"
					}
					fmt.Printf("[%s] %s %s\n", timeStr, icon, log.Message)
				}
				lastCount = len(logs)
			}

			// 检查任务是否完成
			task = engine.GetTask(taskID)
			if task.Status == workflow.StatusCompleted || task.Status == workflow.StatusFailed {
				fmt.Println()
				fmt.Printf("🏁 任务已%s\n", map[workflow.Status]string{
					workflow.StatusCompleted: "完成",
					workflow.StatusFailed:    "失败",
				}[task.Status])
				break
			}

			time.Sleep(500 * time.Millisecond)
		}
	} else {
		// 显示历史日志
		logs := gBoss.GetTaskLogs(taskID, limit)
		if len(logs) == 0 {
			fmt.Println("📭 暂无日志")
		} else {
			for _, log := range logs {
				timeStr := log.Time.Format("15:04:05")
				icon := "📝"
				if log.Level == "error" {
					icon = "❌"
				} else if log.Level == "success" {
					icon = "✅"
				}
				fmt.Printf("[%s] %s %s\n", timeStr, icon, log.Message)
			}
		}

		fmt.Println()
		fmt.Printf("💡 共 %d 条日志，使用 'watch %s -f' 实时跟踪\n", len(logs), taskID[:8])
	}

	fmt.Println()
}

func handleArtifacts(engine *workflow.Engine, session *Session, wsManager *workspace.Manager) {
	project := session.GetProject()
	if project == nil {
		fmt.Println("❌ 未选择项目，先用 'project <ID>' 进入")
		return
	}

	fmt.Println("\n📦 产出物列表:")

	// 从工作空间读取
	if wsManager != nil && project.WorkspaceDir != "" {
		artifacts := wsManager.ListAllArtifacts(project.Name, project.ID)
		if len(artifacts) == 0 {
			fmt.Println("   📭 暂无文件产出物")
		} else {
			for stage, files := range artifacts {
				fmt.Printf("\n   📁 %s/\n", stage)
				for _, file := range files {
					fmt.Printf("      📄 %s\n", file)
				}
			}
		}
	}

	// 内存中的产物
	if len(project.Artifacts) > 0 {
		fmt.Println("\n   💾 内存产物:")
		for name := range project.Artifacts {
			fmt.Printf("      📄 %s (用 'show %s' 查看)\n", name, name)
		}
	}

	fmt.Println()
}

func handleShow(engine *workflow.Engine, session *Session, parts []string) {
	if len(parts) < 2 {
		fmt.Println("用法: show <产出物名称>")
		fmt.Println("例如:")
		fmt.Println("  show prd          # 查看需求文档")
		fmt.Println("  show design       # 查看设计文档")
		fmt.Println("  show code         # 查看代码（开发阶段）")
		fmt.Println("  show main.go      # 查看特定代码文件")
		return
	}

	project := session.GetProject()
	if project == nil {
		fmt.Println("❌ 未选择项目，先用 'project <ID>' 进入")
		return
	}

	name := parts[1]

	// 映射常用名称到阶段
	stageMap := map[string]int{
		"prd":        1,
		"requirement": 1,
		"design":     2,
		"review":     3,
		"code":       4,
		"develop":    4,
		"test":       5,
		"deploy":     6,
	}

	// 如果名称是阶段名，显示该阶段的主文档
	if stageNum, ok := stageMap[name]; ok {
		if content, err := gWsManager.ReadDocument(project.Name, project.ID, stageNum); err == nil && content != "" {
			names := workspace.StageArtifacts[stageNum]
			fmt.Printf("\n📄 %s (%s)\n", names.Document, workspace.StageDirName(stageNum))
			fmt.Println(strings.Repeat("═", 60))
			fmt.Println(content)
			fmt.Println(strings.Repeat("═", 60))
			fmt.Println()
			return
		}
		// 如果没有文档，显示代码文件列表
		files, _ := gWsManager.ListStageFiles(project.Name, project.ID, stageNum)
		if len(files) > 0 {
			fmt.Printf("\n📁 %s/ 下的文件:\n", workspace.StageDirName(stageNum))
			for _, f := range files {
				fmt.Printf("   📄 %s\n", f)
			}
			fmt.Println("\n用 'show <文件名>' 查看具体内容")
		} else {
			fmt.Printf("📭 %s 暂无产出物\n", workspace.StageDirName(stageNum))
		}
		return
	}

	// 尝试作为文件名在各阶段查找
	if gWsManager != nil {
		for stageNum := 1; stageNum <= 6; stageNum++ {
			// 尝试作为主文档读取
			names := workspace.StageArtifacts[stageNum]
			if names.Document == name {
				if content, err := gWsManager.ReadDocument(project.Name, project.ID, stageNum); err == nil && content != "" {
					fmt.Printf("\n📄 %s (%s)\n", name, workspace.StageDirName(stageNum))
					fmt.Println(strings.Repeat("═", 60))
					fmt.Println(content)
					fmt.Println(strings.Repeat("═", 60))
					fmt.Println()
					return
				}
			}

			// 尝试作为代码文件读取
			if content, err := gWsManager.ReadCodeFile(project.Name, project.ID, stageNum, name); err == nil && content != "" {
				fmt.Printf("\n📄 %s (%s)\n", name, workspace.StageDirName(stageNum))
				fmt.Println(strings.Repeat("═", 60))
				fmt.Println(content)
				fmt.Println(strings.Repeat("═", 60))
				fmt.Println()
				return
			}
		}
	}

	// 回退到内存中的产物（兼容旧数据）
	content, ok := project.Artifacts[name]
	if ok {
		fmt.Printf("\n📄 %s (内存缓存)\n", name)
		fmt.Println(strings.Repeat("═", 60))
		if data, ok := content.(map[string]any); ok {
			jsonData, _ := json.MarshalIndent(data, "", "  ")
			fmt.Println(string(jsonData))
		} else {
			fmt.Printf("%v\n", content)
		}
		fmt.Println(strings.Repeat("═", 60))
		fmt.Println()
		return
	}

	fmt.Printf("❌ 产出物 '%s' 不存在\n", name)
	fmt.Println("\n可用命令:")
	fmt.Println("  show prd      - 需求文档")
	fmt.Println("  show design   - 设计文档")
	fmt.Println("  show code     - 开发产物")
	fmt.Println("  show main.go  - 代码文件")
	fmt.Println("\n或用 'artifacts' 查看所有产出物")
}

func handleApprove(engine *workflow.Engine, session *Session, parts []string) {
	if len(parts) < 2 {
		fmt.Println("用法: approve <任务ID>")
		return
	}
	inputID := parts[1]
	taskID := resolveTaskID(engine, inputID, session)
	if taskID == "" {
		fmt.Println("❌ 任务不存在（或多个匹配）")
		return
	}
	task := engine.GetTask(taskID)

	engine.CompleteTask(taskID, task.Output)
	fmt.Printf("✅ 已批准任务: %s\n", taskID[:12])
	fmt.Println("⏳ 工作流正在推进到下一阶段...")
}

func handleReject(engine *workflow.Engine, session *Session, parts []string) {
	if len(parts) < 3 {
		fmt.Println("用法: reject <任务ID> <原因>")
		return
	}
	inputID := parts[1]
	reason := parts[2]
	taskID := resolveTaskID(engine, inputID, session)
	if taskID == "" {
		fmt.Println("❌ 任务不存在（或多个匹配）")
		return
	}
	if err := engine.RejectTask(taskID, reason); err != nil {
		fmt.Printf("❌ 驳回失败: %v\n", err)
	} else {
		fmt.Printf("🔄 已驳回任务: %s\n", taskID[:12])
		fmt.Printf("   原因: %s\n", reason)
	}
}

// resolveTaskID 解析任务ID（支持短ID前缀，优先当前项目）
func resolveTaskID(engine *workflow.Engine, inputID string, session *Session) string {
	// 先尝试完整匹配
	if engine.GetTask(inputID) != nil {
		return inputID
	}

	// 获取要搜索的任务列表
	var tasks []*workflow.Task
	if project := session.GetProject(); project != nil {
		for _, stageTasks := range project.Tasks {
			tasks = append(tasks, stageTasks...)
		}
	} else {
		tasks = engine.GetAllTasks()
	}

	// 前缀匹配
	var matched *workflow.Task
	for _, t := range tasks {
		if strings.HasPrefix(t.ID, inputID) {
			if matched != nil {
				return "" // 多个匹配
			}
			matched = t
		}
	}
	if matched != nil {
		return matched.ID
	}
	return ""
}

// setupEventListeners 设置事件监听
func setupEventListeners(engine *workflow.Engine, mq *MessageQueue, session *Session, wsManager *workspace.Manager, boss *master.Manager) {
	engine.On("project.created", func(data interface{}) {
		project := data.(*workflow.Project)
		mq.Push(fmt.Sprintf("🎉 新项目启动: %s", project.Name))
		if project.WorkspaceDir != "" {
			mq.Push(fmt.Sprintf("   📁 工作空间: %s", project.WorkspaceDir))
		}
	})

	engine.On("task.created", func(data interface{}) {
		task := data.(*workflow.Task)
		mq.Push(fmt.Sprintf("📋 新任务: [%s] %s", getStageName(task.Stage), task.Name))
		
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
		mq.Push(fmt.Sprintf("✅ 任务完成: %s [%s]", task.Name, getStageName(task.Stage)))

		// 保存产物到工作空间
		if task.Output != nil && wsManager != nil {
			project := engine.GetProject(task.ProjectID)
			if project != nil && project.WorkspaceDir != "" {
				stageNum := getStageNumber(task.Stage)
				filename := fmt.Sprintf("%s-output.json", task.Stage)

				// 将输出转为 JSON
				content, err := json.MarshalIndent(task.Output, "", "  ")
				if err == nil {
					err = wsManager.WriteFile(project.Name, project.ID, stageNum, filename, content)
					if err == nil {
						mq.Push(fmt.Sprintf("   💾 已保存到: %s/%s", getStageDirName(stageNum), filename))
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

func getStageNumber(stage workflow.Stage) int {
	stages := map[workflow.Stage]int{
		workflow.StageRequirement: 1,
		workflow.StageDesign:      2,
		workflow.StageReview:      3,
		workflow.StageDevelop:     4,
		workflow.StageTest:        5,
		workflow.StageDeploy:      6,
	}
	if n, ok := stages[stage]; ok {
		return n
	}
	return 0
}

// resumeTasks 恢复未完成的任务
func resumeTasks(engine *workflow.Engine, boss *master.Manager) {
	tasks := engine.GetAllTasks()

	var resumed int
	for _, task := range tasks {
		// 只恢复已分配但未完成的任务
		if task.Status == workflow.StatusAssigned || task.Status == workflow.StatusInProgress {
			role := getRoleForStage(task.Stage)
			if role != "" {
				name := task.Name
				if len(name) > 20 {
					name = name[:20]
				}
				fmt.Printf("🔄 恢复任务: %s [%s] → %s\n", name, getStageName(task.Stage), role)
				// 清空旧的 Assignee，强制重新分配
				if err := boss.ReassignTask(task.ID, role); err != nil {
					fmt.Printf("   ⚠️ 恢复失败: %v\n", err)
				} else {
					resumed++
				}
			}
		}
	}

	if resumed > 0 {
		fmt.Printf("\n✅ 已恢复 %d 个任务\n\n", resumed)
	}
}

// getRoleForStage 根据阶段获取角色
func getRoleForStage(stage workflow.Stage) string {
	roles := map[workflow.Stage]string{
		workflow.StageRequirement: "product",
		workflow.StageDesign:      "developer",
		workflow.StageReview:      "product",
		workflow.StageDevelop:     "developer",
		workflow.StageTest:        "tester",
		workflow.StageDeploy:      "developer",
	}
	if role, ok := roles[stage]; ok {
		return role
	}
	return ""
}

func getStageDirName(stageNum int) string {
	names := map[int]string{
		1: "01-requirement",
		2: "02-design",
		3: "03-review",
		4: "04-develop",
		5: "05-test",
		6: "06-deploy",
	}
	if name, ok := names[stageNum]; ok {
		return name
	}
	return "docs"
}

func getStageName(stage workflow.Stage) string {
	names := map[workflow.Stage]string{
		workflow.StageRequirement: "需求分析",
		workflow.StageDesign:      "系统设计",
		workflow.StageReview:      "设计评审",
		workflow.StageDevelop:     "功能开发",
		workflow.StageTest:        "测试验证",
		workflow.StageDeploy:      "部署上线",
	}
	if name, ok := names[stage]; ok {
		return name
	}
	return string(stage)
}

func getStatusIcon(status workflow.Status) string {
	icons := map[workflow.Status]string{
		workflow.StatusPending:    "⏳",
		workflow.StatusAssigned:   "👤",
		workflow.StatusInProgress: "🔄",
		workflow.StatusCompleted:  "✅",
		workflow.StatusRejected:   "❌",
		workflow.StatusFailed:     "💥",
	}
	if icon, ok := icons[status]; ok {
		return icon
	}
	return "❓"
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
