package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"cyberteam/internal/common"
	"cyberteam/internal/master"
	"cyberteam/internal/session"
	"cyberteam/internal/workflow"
	"cyberteam/internal/workspace"
)

// 内置命令列表（会议模式下需要区分的命令）
var builtinCommands = map[string]bool{
	"new": true, "projects": true, "ls": true,
	"project": true, "cd": true,
	"..":     true,
	"status": true, "st": true,
	"tasks":     true,
	"watch":     true,
	"artifacts": true, "art": true,
	"show": true, "cat": true,
	"approve": true, "ok": true,
	"reject": true, "no": true,
	"team":    true,
	"meeting": true, "mtg": true, "m": true,
	"chat": true, "c": true,
	"mcp":  true,
	"help": true, "h": true,
	"exit": true, "quit": true,
}

// isBuiltinCommand 检查是否是内置命令
func isBuiltinCommand(cmd string) bool {
	return builtinCommands[cmd]
}

// processInput 处理用户输入
func processInput(line string, engine *workflow.Engine, boss *master.Manager, sess *session.Session) {
	parts := strings.SplitN(line, " ", 3)
	cmd := parts[0]

	// 私聊模式：.. 退出私聊
	if sess.InPrivateChat() && cmd == ".." {
		sess.ExitPrivateChat()
		fmt.Println("(退出私聊)")
		return
	}

	// 私聊模式：非命令输入 = 发送私聊消息
	if sess.InPrivateChat() && !isBuiltinCommand(cmd) {
		handlePrivateMessage(sess, line)
		return
	}

	// 会议模式下，非命令输入 = 直接发言
	if sess.InMeeting() && !isBuiltinCommand(cmd) {
		if strings.HasPrefix(line, "@") {
			// @开头 = 点名发言
			handleDirectMention(sess, line)
		} else {
			// 自由发言
			handleDirectSay(sess, line)
		}
		return
	}

	switch cmd {
	case "new":
		handleNew(engine, boss, sess, parts)
	case "projects", "ls":
		handleProjects(engine, sess)
	case "project", "cd":
		handleProject(engine, sess, parts)
	case "..":
		handleExitProject(sess)
	case "status", "st":
		handleStatus(engine, sess)
	case "tasks":
		handleTasks(engine, sess)
	case "watch":
		handleWatch(engine, sess, parts)
	case "artifacts", "art":
		handleArtifacts(engine, sess, gWsManager)
	case "show", "cat":
		handleShow(engine, sess, parts)
	case "approve", "ok":
		handleApprove(engine, sess, parts)
	case "reject", "no":
		handleReject(engine, sess, parts)
	case "team":
		boss.ShowTeam()
	case "meeting", "mtg", "m":
		handleMeeting(sess, parts)
	case "chat", "c":
		handleChat(sess, parts[1:])
	case "mcp":
		handleMCPStatus()
	case "help", "h":
		printHelp()
	case "exit", "quit":
		fmt.Println("\n👋 正在关闭公司...")
		boss.Shutdown()
		fmt.Println("再见！")
		os.Exit(0)
	default:
		fmt.Println("未知命令，输入 'help' 查看帮助")
	}
}

func handleMCPStatus() {
	if gMCPManager == nil {
		fmt.Println("❌ MCP 未初始化")
		return
	}

	fmt.Println("\n🛠️ MCP 工具状态:")
	fmt.Println(strings.Repeat("-", 40))

	status := gMCPManager.GetServerStatus()
	if len(status) == 0 {
		fmt.Println("  无可用 MCP Server")
		return
	}

	for name, s := range status {
		icon := "❌"
		if s == "ready" {
			icon = "✅"
		}
		fmt.Printf("  %s %s: %s\n", icon, name, s)
	}

	fmt.Println()
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
	fmt.Println()
	fmt.Println("🗣️ 会议命令:")
	fmt.Println("  meeting start <主题> [--mode free|round]  开始会议")
	fmt.Println("  meeting list                              列出会议")
	fmt.Println("  meeting join <ID>                         加入会议")
	fmt.Println("  meeting end                               结束会议")
	fmt.Println()
	fmt.Println("💡 提示: 进入会议后，直接输入内容即可发言")
	mentionExample := "@<name>"
	if names := getOnlineStaffNames(); len(names) > 0 {
		mentionExample = "@" + names[0]
	}
	fmt.Printf("        %-16s 你的问题  - 点名指定人回答\n", mentionExample)
	fmt.Println("        大家好              - 自由发言（随机人回复）")
	fmt.Println()
	fmt.Println("💬 私聊命令:")
	fmt.Println("  chat <name>             和指定员工私聊")
	fmt.Println("  ..                      退出私聊")
	fmt.Println()
	fmt.Println("🛠️ MCP 工具:")
	fmt.Println("  mcp                     查看 MCP 工具状态")
	fmt.Println()
}

func handleNew(engine *workflow.Engine, boss *master.Manager, sess *session.Session, parts []string) {
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
		sess.SetProject(project)
		fmt.Printf("\n🔀 已进入项目 [%s]\n", name)
	}
}

func handleProjects(engine *workflow.Engine, sess *session.Session) {
	projects := engine.GetAllProjects()
	if len(projects) == 0 {
		fmt.Println("📭 暂无项目")
		return
	}

	current := sess.GetProject()

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
			common.Truncate(p.Name, 18),
			p.Status,
			common.GetStageName(p.CurrentStage))
	}
	fmt.Println()
	fmt.Println("💡 用 'project <ID>' 或 'cd <ID>' 进入项目")
}

func handleProject(engine *workflow.Engine, sess *session.Session, parts []string) {
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

	sess.SetProject(project)
	fmt.Printf("\n🔀 已进入项目 [%s]\n", project.Name)
	showProjectStatus(project)
}

func handleExitProject(sess *session.Session) {
	sess.SetProject(nil)
	fmt.Println("📤 已退出项目")
}

func handleStatus(engine *workflow.Engine, sess *session.Session) {
	project := sess.GetProject()
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
	fmt.Printf("   当前阶段: %s\n", common.GetStageName(project.CurrentStage))
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
		statusIcon := common.GetStatusIcon(task.Status)
		fmt.Printf("   [%s] %s %s (%s)\n", common.GetStageName(stage), statusIcon, task.Name, assignee)
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

func handleTasks(engine *workflow.Engine, sess *session.Session) {
	project := sess.GetProject()

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
			common.Truncate(t.Name, 18),
			common.GetStageName(t.Stage),
			t.Status,
			assignee)
	}
	fmt.Println()
	if project != nil {
		fmt.Println("💡 用 'approve <ID>' 批准任务，'reject <ID> <原因>' 驳回")
	}
}

func handleWatch(engine *workflow.Engine, sess *session.Session, parts []string) {
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
	taskID := resolveTaskID(engine, inputID, sess)
	if taskID == "" {
		fmt.Println("❌ 任务不存在（或多个匹配）")
		return
	}

	task := engine.GetTask(taskID)
	if task == nil {
		fmt.Println("❌ 任务不存在")
		return
	}

	// 解析选项：parts[2] 是未拆分的尾部，需要 Fields 重新分词
	follow := false
	limit := 50

	var flagArgs []string
	if len(parts) > 2 {
		flagArgs = strings.Fields(parts[2])
	}
	for i := 0; i < len(flagArgs); i++ {
		switch flagArgs[i] {
		case "-f", "--follow":
			follow = true
		case "-n":
			if i+1 < len(flagArgs) {
				if n, err := strconv.Atoi(flagArgs[i+1]); err == nil && n > 0 {
					limit = n
					i++
				}
			}
		}
	}

	// 显示任务信息
	fmt.Printf("\n👀 观察任务: %s [%s]\n", task.Name, common.GetStageName(task.Stage))
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
			if task == nil || task.Status == workflow.StatusCompleted || task.Status == workflow.StatusFailed {
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

func handleArtifacts(engine *workflow.Engine, sess *session.Session, wsManager *workspace.Manager) {
	project := sess.GetProject()
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

func handleShow(engine *workflow.Engine, sess *session.Session, parts []string) {
	if len(parts) < 2 {
		fmt.Println("用法: show <产出物名称>")
		fmt.Println("例如:")
		fmt.Println("  show prd          # 查看需求文档")
		fmt.Println("  show design       # 查看设计文档")
		fmt.Println("  show code         # 查看代码（开发阶段）")
		fmt.Println("  show main.go      # 查看特定代码文件")
		return
	}

	project := sess.GetProject()
	if project == nil {
		fmt.Println("❌ 未选择项目，先用 'project <ID>' 进入")
		return
	}

	name := parts[1]

	// 映射常用名称到阶段
	stageMap := map[string]int{
		"prd":         1,
		"requirement": 1,
		"design":      2,
		"review":      3,
		"code":        4,
		"develop":     4,
		"test":        5,
		"deploy":      6,
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

func handleApprove(engine *workflow.Engine, sess *session.Session, parts []string) {
	if len(parts) < 2 {
		fmt.Println("用法: approve <任务ID>")
		return
	}
	inputID := parts[1]
	taskID := resolveTaskID(engine, inputID, sess)
	if taskID == "" {
		fmt.Println("❌ 任务不存在（或多个匹配）")
		return
	}
	task := engine.GetTask(taskID)
	if task == nil {
		fmt.Println("❌ 任务不存在")
		return
	}

	engine.CompleteTask(taskID, task.Output)
	fmt.Printf("✅ 已批准任务: %s\n", taskID[:12])
	fmt.Println("⏳ 工作流正在推进到下一阶段...")
}

func handleReject(engine *workflow.Engine, sess *session.Session, parts []string) {
	if len(parts) < 3 {
		fmt.Println("用法: reject <任务ID> <原因>")
		return
	}
	inputID := parts[1]
	reason := parts[2]
	taskID := resolveTaskID(engine, inputID, sess)
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
func resolveTaskID(engine *workflow.Engine, inputID string, sess *session.Session) string {
	// 先尝试完整匹配
	if engine.GetTask(inputID) != nil {
		return inputID
	}

	// 获取要搜索的任务列表
	var tasks []*workflow.Task
	if project := sess.GetProject(); project != nil {
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
