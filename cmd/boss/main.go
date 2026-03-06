package main

import (
	"cyberteam/internal/master"
	"cyberteam/internal/mcp"
	"cyberteam/internal/meeting"
	"cyberteam/internal/profile"
	"cyberteam/internal/storage"
	"cyberteam/internal/workflow"
	"cyberteam/internal/workspace"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chzyer/readline"
)

// 全局变量（简化命令处理）
var gWsManager *workspace.Manager
var gBoss *master.Manager
var gMeetingRoom *meeting.Room
var gMCPManager *mcp.Manager

// Session 当前会话状态
type Session struct {
	mu             sync.RWMutex
	currentProject *workflow.Project
	currentMeeting *meeting.Meeting
	privateChat    *PrivateChat // 当前私聊对象
}

// PrivateChat 私聊状态
type PrivateChat struct {
	With      string    // 对方名字
	StartedAt time.Time // 开始时间
	History   []ChatMessage // 聊天记录
}

// ChatMessage 单条聊天消息
type ChatMessage struct {
	From      string
	Content   string
	Timestamp time.Time
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

func (s *Session) SetMeeting(m *meeting.Meeting) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentMeeting = m
}

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
	fmt.Println("🏢 CyberTeam")
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

	// 加载 Boss Profile
	bossProfilePath := filepath.Join(rootDir, "cmd/boss/PROFILE.md")
	if prof, err := profile.Load(bossProfilePath); err == nil {
		// 只取描述第一行
		desc := strings.Split(prof.Description, "\n")[0]
		if len(desc) > 50 {
			desc = desc[:50] + "..."
		}
		fmt.Printf("👤 Boss: %s - %s\n", prof.Name, desc)
	}

	// 设置 Staff 消息回调
	boss.SetMessageCallback(func(staffID, msgType, content string) {
		if msgType == "meeting_reply" {
			// 会议回复直接显示（Staff 的发言），添加颜色
			coloredContent := colorizeMeetingReply(content)
			msgQueue.Push(coloredContent)
		} else {
			// 普通消息也添加颜色
			coloredContent := colorizeStaffMessage(staffID, content)
			msgQueue.Push(coloredContent)
		}
	})

	// 召集员工
	staffs := []struct {
		role   string
		name   string
		binary string
		emoji  string
	}{
		{"product", "Sarah", filepath.Join(rootDir, "cmd/staff/product/product"), "👩‍💼"},
		{"developer", "Alex", filepath.Join(rootDir, "cmd/staff/developer/developer"), "👨‍💻"},
		{"tester", "Mia", filepath.Join(rootDir, "cmd/staff/tester/tester"), "🧪"},
	}

	fmt.Println("\n🎯 正在召集团队...")
	descriptions := map[string]string{
		"Sarah": "产品经理（咖啡成瘾，纸笔画图派）",
		"Alex":  "架构师（代码洁癖，键盘收藏家）",
		"Mia":   "测试专家（找茬天赋，清单强迫症）",
	}
	for _, s := range staffs {
		if _, err := boss.HireStaff(s.role, s.name, s.binary); err != nil {
			fmt.Printf("❌ %s %s 打卡失败: %v\n", s.emoji, s.name, err)
		} else {
			fmt.Printf("   %s %s - %s\n", s.emoji, s.name, descriptions[s.name])
			time.Sleep(200 * time.Millisecond)
		}
	}

	fmt.Println("\n✅ 全员到齐，准备开工！")
	time.Sleep(500 * time.Millisecond)

	// 初始化会议室
	meetingDir := filepath.Join(rootDir, "meetings")
	gMeetingRoom = meeting.NewRoom(meetingDir)
	fmt.Println("✅ 会议室就绪！")

	// 初始化 MCP 管理器
	mcpConfigPath := filepath.Join(rootDir, "config", "mcp.yaml")
	mcpManager, err := mcp.NewManager(mcpConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ MCP 配置加载失败: %v\n", err)
	} else {
		boss.SetMCPManager(mcpManager)
		if err := mcpManager.StartAll(); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️ MCP 启动失败: %v\n", err)
		} else {
			fmt.Println("✅ MCP 工具就绪！")
			// 显示可用工具
			for name, status := range mcpManager.GetServerStatus() {
				fmt.Printf("   - %s: %s\n", name, status)
			}
		}
		gMCPManager = mcpManager
	}

	// 设置事件监听（需要在 boss 创建后）
	setupEventListeners(engine, msgQueue, session, wsManager, boss)

	// 恢复未完成的任务
	resumeTasks(engine, boss)

	// 显示帮助
	printHelp()

	// 交互式命令行（使用 readline 支持中文和 Ctrl+C）
	rl, err := readline.New(session.GetPrompt())
	if err != nil {
		// 降级到标准输入（使用 bufio 简单回退）
		fmt.Println("⚠️ 读取终端失败，使用标准输入模式")
		fmt.Println("提示: 安装 readline 可获得更好的输入体验")
		fmt.Print(session.GetPrompt())

		// 简单标准输入回退
		var input string
		for {
			if _, err := fmt.Scanln(&input); err != nil {
				return
			}
			processInput(strings.TrimSpace(input), engine, boss, session)
			fmt.Print(session.GetPrompt())
		}
	}
	defer rl.Close()

	// 启动消息显示 goroutine（在 rl 创建后，以便调用 Refresh）
	go func() {
		for {
			msgQueue.Wait()
			msgs := msgQueue.PopAll()
			if len(msgs) > 0 {
				// 清除当前行并打印所有消息
				fmt.Printf("\r\033[K")
				for _, msg := range msgs {
					// 使用简洁格式，时间右对齐灰色显示
					timeStr := time.Now().Format("15:04:05")
					// 格式: 消息内容 ... 时间(灰色)
					const totalWidth = 80
					lineLen := len(msg)
					spaces := totalWidth - lineLen - len(timeStr)
					if spaces < 1 {
						spaces = 1
					}
					fmt.Printf("%s%s%s%s%s\n", msg, strings.Repeat(" ", spaces), ColorGray, timeStr, ColorReset)
				}
				// 恢复输入行
				rl.Refresh()
			}
		}
	}()

	// 设置 Ctrl+C 处理
	var doubleCtrlC bool

	for {
		line, err := rl.Readline()

		// 处理 Ctrl+C
		if err == readline.ErrInterrupt {
			if doubleCtrlC {
				// 第二次 Ctrl+C，优雅退出
				fmt.Println("\n👋 正在关闭公司...")
				boss.Shutdown()
				fmt.Println("再见！")
				return
			}
			// 第一次 Ctrl+C，清空当前行
			doubleCtrlC = true
			fmt.Println("\n(输入已清空)")
			// readline 会自动刷新提示符，不需要手动打印
			continue
		}
		doubleCtrlC = false // 重置标志

		if err != nil {
			// EOF 或其他错误，退出
			if err.Error() == "EOF" {
				fmt.Println("\n👋 再见！")
			}
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		processInput(line, engine, boss, session)
		// 更新提示符（会议状态可能改变）
		rl.SetPrompt(session.GetPrompt())
	}
}

// 内置命令列表（会议模式下需要区分的命令）
var builtinCommands = map[string]bool{
	"new": true, "projects": true, "ls": true,
	"project": true, "cd": true,
	"..": true,
	"status": true, "st": true,
	"tasks": true,
	"watch": true,
	"artifacts": true, "art": true,
	"show": true, "cat": true,
	"approve": true, "ok": true,
	"reject": true, "no": true,
	"team": true,
	"meeting": true, "mtg": true, "m": true,
	"chat": true, "c": true,
	"help": true, "h": true,
	"exit": true, "quit": true,
}

// isBuiltinCommand 检查是否是内置命令
func isBuiltinCommand(cmd string) bool {
	return builtinCommands[cmd]
}

// processInput 处理用户输入
func processInput(line string, engine *workflow.Engine, boss *master.Manager, session *Session) {
	parts := strings.SplitN(line, " ", 3)
	cmd := parts[0]

	// 私聊模式：.. 退出私聊
	if session.InPrivateChat() && cmd == ".." {
		session.ExitPrivateChat()
		fmt.Println("(退出私聊)")
		return
	}

	// 私聊模式：非命令输入 = 发送私聊消息
	if session.InPrivateChat() && !isBuiltinCommand(cmd) {
		handlePrivateMessage(session, line)
		return
	}

	// 会议模式下，非命令输入 = 直接发言
	if session.InMeeting() && !isBuiltinCommand(cmd) {
		if strings.HasPrefix(line, "@") {
			// @开头 = 点名发言
			handleDirectMention(session, line)
		} else {
			// 自由发言
			handleDirectSay(session, line)
		}
		return
	}

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
	case "meeting", "mtg", "m":
		handleMeeting(session, parts)
	case "chat", "c":
		handleChat(session, parts[1:])
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
	fmt.Println("        @Alex 你的问题    - 点名指定人回答")
	fmt.Println("        大家好              - 自由发言（随机人回复）")
	fmt.Println()
	fmt.Println("💬 私聊命令:")
	fmt.Println("  chat <name>             和指定员工私聊")
	fmt.Println("  ..                      退出私聊")
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

// ==================== Meeting Commands ====================

func handleMeeting(session *Session, parts []string) {
	if len(parts) < 2 {
		fmt.Println("❌ 用法: meeting <start|list|join|say|ask|end|transcript>")
		return
	}

	subCmd := parts[1]
	args := parts[2:]

	switch subCmd {
	case "start":
		handleMeetingStart(session, args)
	case "list", "ls":
		handleMeetingList()
	case "join":
		handleMeetingJoin(session, args)
	case "end":
		handleMeetingEnd(session)
	case "transcript", "log":
		handleMeetingTranscript(session)
	default:
		fmt.Printf("❌ 未知会议命令: %s\n", subCmd)
	}
}

func handleMeetingStart(session *Session, args []string) {
	if len(args) < 1 {
		fmt.Println("❌ 用法: meeting start <topic> [--with staff1,staff2] [--mode free|round|boss]")
		return
	}

	topic := args[0]
	mode := meeting.ModeFree
	participants := []string{"product", "developer", "tester"}

	// 解析参数
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--with":
			if i+1 < len(args) {
				participants = strings.Split(args[i+1], ",")
				i++
			}
		case "--mode":
			if i+1 < len(args) {
				mode = meeting.Mode(args[i+1])
				i++
			}
		}
	}

	// 创建会议
	mtg, err := gMeetingRoom.CreateMeeting(topic, mode, participants, "boss")
	if err != nil {
		fmt.Printf("❌ 创建会议失败: %v\n", err)
		return
	}

	// 进入会议模式
	session.SetMeeting(mtg)

	// 注册消息回调（流式显示）
	gMeetingRoom.OnMessage(func(meetingID string, msg meeting.Message) {
		if meetingID != mtg.ID {
			return
		}
		// 不显示 Boss 自己的消息（避免重复）
		if msg.From == "boss" {
			return
		}
		displayMeetingMessage(msg, session)
	})

	fmt.Printf("\n🎤 会议 [%s] 开始\n", mtg.Topic)
	fmt.Printf("📋 ID: %s\n", mtg.ID)
	fmt.Printf("👥 参与者: %s\n", strings.Join(participants, ", "))
	fmt.Printf("📌 模式: %s\n", mode)
	fmt.Println("\n💡 直接输入发言，或 @某人 点名:")
	fmt.Println("   大家好           - 自由发言")
	fmt.Println("   @Alex 评估一下  - 点名提问")
	fmt.Println("   meeting end      - 结束会议")
}

func handleMeetingList() {
	meetings := gMeetingRoom.ListMeetings()

	if len(meetings) == 0 {
		fmt.Println("📭 暂无会议")
		return
	}

	fmt.Println("\n📋 会议列表")
	fmt.Println(strings.Repeat("-", 80))
	for _, m := range meetings {
		status := "🟢"
		if m.Status == meeting.StatusCompleted {
			status = "✅"
		}
		fmt.Printf("%s [%s] %s | 模式: %s | 消息: %d条 | %s\n",
			status, m.ID[:12], m.Topic, m.Mode, len(m.Messages), m.Status)
	}
	fmt.Println()
}

func handleMeetingJoin(session *Session, args []string) {
	if len(args) < 1 {
		fmt.Println("❌ 用法: meeting join <id>")
		return
	}

	id := args[0]
	mtg, ok := gMeetingRoom.GetMeeting(id)
	if !ok {
		// 尝试前缀匹配
		meetings := gMeetingRoom.ListMeetings()
		for _, m := range meetings {
			if strings.HasPrefix(m.ID, id) {
				mtg = m
				ok = true
				break
			}
		}
	}
	if !ok {
		fmt.Printf("❌ 会议不存在: %s\n", id)
		return
	}

	session.SetMeeting(mtg)

	fmt.Printf("\n🎤 已进入会议 [%s]\n", mtg.Topic)
	fmt.Printf("📋 状态: %s | 消息: %d条\n", mtg.Status, len(mtg.Messages))
	fmt.Println("\n💡 直接输入发言，或 @某人 点名")

	if len(mtg.Messages) > 0 {
		fmt.Println("\n📜 最近消息:")
		start := len(mtg.Messages) - 5
		if start < 0 {
			start = 0
		}
		for _, msg := range mtg.Messages[start:] {
			displayMeetingMessage(msg, session)
		}
	}
}

func handleMeetingSay(session *Session, args []string) {
	mtg := session.GetMeeting()
	if mtg == nil {
		fmt.Println("❌ 当前不在会议中，使用 'meeting start <topic>' 开始")
		return
	}

	content := strings.Join(args, " ")
	if content == "" {
		fmt.Println("❌ 发言内容不能为空")
		return
	}

	_, err := gMeetingRoom.AddMessage(mtg.ID, "boss", meeting.MsgText, content)
	if err != nil {
		fmt.Printf("❌ 发送失败: %v\n", err)
		return
	}

	// 检查是否 @ 了某人，并发送消息给对应的 Staff
	mentioned := extractMentions(content)
	transcript := mtg.GetTranscript()
	if len(mentioned) > 0 {
		for _, role := range mentioned {
			go gBoss.SendMeetingMessage(role, mtg.ID, "boss", content, true, transcript)
		}
	} else {
		// 自由模式下，随机选择 1-2 人回复（避免太吵）
		transcript := mtg.GetTranscript()
		go gBoss.BroadcastMeetingMessageRandom(mtg.ID, "boss", content, 2, transcript)
	}
}

func handleMeetingAsk(session *Session, args []string) {
	mtg := session.GetMeeting()
	if mtg == nil {
		fmt.Println("❌ 当前不在会议中")
		return
	}

	if len(args) < 2 {
		fmt.Println("❌ 用法: ask <staff> <question>")
		return
	}

	staff := args[0]
	question := strings.Join(args[1:], " ")

	content := fmt.Sprintf("@%s %s", staff, question)
	_, err := gMeetingRoom.AddMessage(mtg.ID, "boss", meeting.MsgMention, content)
	if err != nil {
		fmt.Printf("❌ 发送失败: %v\n", err)
		return
	}

	// 将名字转换为 role
	staffRole := staff
	if role, ok := nameToRole[staff]; ok {
		staffRole = role
	}

	// 获取会议历史
	transcript := mtg.GetTranscript()

	// 发送消息给指定的 Staff
	go gBoss.SendMeetingMessage(staffRole, mtg.ID, "boss", question, true, transcript)
	fmt.Printf("🎯 已向 @%s 提问\n", staff)
}

// handleDirectSay 直接自由发言（方案 C）
func handleDirectSay(session *Session, content string) {
	mtg := session.GetMeeting()
	if mtg == nil {
		return
	}

	if content == "" {
		return
	}

	_, err := gMeetingRoom.AddMessage(mtg.ID, "boss", meeting.MsgText, content)
	if err != nil {
		fmt.Printf("❌ 发送失败: %v\n", err)
		return
	}

	// 自由模式下，随机选择 1-2 人回复
	transcript := mtg.GetTranscript()
	go gBoss.BroadcastMeetingMessageRandom(mtg.ID, "boss", content, 2, transcript)
}

// handleDirectMention 直接 @ 点名发言（方案 C）
func handleDirectMention(session *Session, line string) {
	mtg := session.GetMeeting()
	if mtg == nil {
		return
	}

	// 解析 @名字 内容
	// 格式: @Alex 内容 或 @Alex @Sarah 内容
	parts := strings.SplitN(line[1:], " ", 2) // 去掉开头的@
	if len(parts) < 1 {
		return
	}

	// 提取所有 @ 的名字
	var names []string
	content := line
	for strings.HasPrefix(content, "@") {
		p := strings.SplitN(content[1:], " ", 2)
		if len(p) >= 1 {
			names = append(names, p[0])
			if len(p) >= 2 {
				content = p[1]
			} else {
				content = ""
				break
			}
		} else {
			break
		}
	}

	if len(names) == 0 {
		return
	}

	// 构建完整内容（包含 @ 标记）
	fullContent := line

	_, err := gMeetingRoom.AddMessage(mtg.ID, "boss", meeting.MsgMention, fullContent)
	if err != nil {
		fmt.Printf("❌ 发送失败: %v\n", err)
		return
	}

	// 获取会议历史
	transcript := mtg.GetTranscript()

	// 发送给所有被 @ 的人
	for _, name := range names {
		staffRole := name
		if role, ok := nameToRole[name]; ok {
			staffRole = role
		}
		go gBoss.SendMeetingMessage(staffRole, mtg.ID, "boss", fullContent, true, transcript)
	}
}

func handleMeetingEnd(session *Session) {
	mtg := session.GetMeeting()
	if mtg == nil {
		fmt.Println("❌ 当前不在会议中")
		return
	}

	fmt.Println("\n📝 正在结束会议并保存记录...")

	// 简单总结
	summary := fmt.Sprintf("会议 [%s] 共 %d 条消息，参与者: %s",
		mtg.Topic, len(mtg.Messages), strings.Join(mtg.Participants, ", "))

	// 结束会议
	if err := gMeetingRoom.EndMeeting(mtg.ID, summary, []string{}); err != nil {
		fmt.Printf("❌ 结束会议失败: %v\n", err)
		return
	}

	fmt.Printf("\n✅ 会议 [%s] 已结束\n", mtg.Topic)
	fmt.Printf("📁 记录保存到: meetings/%s/\n", mtg.ID)

	// 退出会议模式
	session.SetMeeting(nil)
}

func handleMeetingTranscript(session *Session) {
	mtg := session.GetMeeting()
	if mtg == nil {
		fmt.Println("❌ 当前不在会议中")
		return
	}

	fmt.Printf("\n📜 会议 [%s] 记录\n", mtg.Topic)
	fmt.Println(strings.Repeat("-", 80))

	for _, msg := range mtg.Messages {
		displayMeetingMessage(msg, session)
	}
}

func displayMeetingMessage(msg meeting.Message, session *Session) {
	// 不显示 Boss 自己的消息（避免重复）
	if msg.From == "boss" {
		return
	}

	timeStr := msg.Timestamp.Format("15:04:05")

	// 获取发送者颜色
	color := getSenderColor(msg.From)
	reset := ColorReset

	// 总宽度（终端宽度减去一些边距）
	const totalWidth = 80

	switch msg.Type {
	case meeting.MsgText, meeting.MsgMention:
		// 格式: 名字: 内容 ... 时间(灰色右对齐)
		prefix := fmt.Sprintf("%s%s%s: ", color, msg.From, reset)
		content := msg.Content
		if len(content) > 50 {
			content = content[:50] + "..."
		}
		// 计算剩余空间给时间
		lineLen := len(msg.From) + 2 + len(content)
		spaces := totalWidth - lineLen - len(timeStr)
		if spaces < 1 {
			spaces = 1
		}
		fmt.Printf("%s%s%s%s%s%s\n", prefix, content, strings.Repeat(" ", spaces), ColorGray, timeStr, ColorReset)
	case meeting.MsgAction:
		fmt.Printf("*%s* %s%s%s\n", msg.Content, ColorGray, timeStr, ColorReset)
	}
}

// getSenderColor 获取发送者的颜色
func getSenderColor(from string) string {
	role := ""
	if r, ok := nameToRole[from]; ok {
		role = r
	}
	// 特殊处理 boss
	if from == "boss" {
		role = "boss"
	}
	if c, ok := roleColors[role]; ok {
		return c
	}
	return ColorWhite
}

// ==================== Private Chat Commands ====================

func handleChat(session *Session, args []string) {
	if len(args) < 1 {
		fmt.Println("❌ 用法: chat <name>")
		fmt.Println("   chat Sarah    - 和 Sarah 私聊")
		fmt.Println("   chat Alex     - 和 Alex 私聊")
		fmt.Println("   chat Mia      - 和 Mia 私聊")
		fmt.Println("   ..            - 退出私聊")
		return
	}

	name := args[0]

	// 检查是否是有效的员工名字
	role, ok := nameToRole[name]
	if !ok {
		fmt.Printf("❌ 未知员工: %s\n", name)
		fmt.Println("   可用: Sarah, Alex, Mia")
		return
	}

	// 检查员工是否在线
	if !gBoss.IsStaffOnline(role) {
		fmt.Printf("❌ %s 当前不在线\n", name)
		return
	}

	// 进入私聊模式
	session.SetPrivateChat(name)

	// 注册私聊消息回调
	gBoss.SetPrivateMessageCallback(func(staffName, content string) {
		// 只显示当前私聊对象的消息
		pc := session.GetPrivateChat()
		if pc != nil && pc.With == staffName {
			color := getSenderColor(staffName)
			fmt.Printf("\n%s%s%s: %s\n", color, staffName, ColorReset, content)
			// 添加到历史记录
			session.AddPrivateChatMessage(staffName, content)
			// 刷新提示符
			fmt.Print(session.GetPrompt())
		}
	})

	fmt.Printf("\n💬 开始和 %s 私聊\n", name)
	fmt.Println("   直接输入消息发送")
	fmt.Println("   .. 退出私聊")
}

func handlePrivateMessage(session *Session, content string) {
	pc := session.GetPrivateChat()
	if pc == nil {
		return
	}

	// 获取对方 role
	role, ok := nameToRole[pc.With]
	if !ok {
		fmt.Println("❌ 私聊对象错误")
		return
	}

	// 添加消息到历史
	session.AddPrivateChatMessage("Kai", content)

	// 获取历史记录
	history := session.GetPrivateChatHistory()

	// 发送私聊消息给 Staff（带上历史）
	go gBoss.SendPrivateMessage(role, "boss", content, history)

	// 本地显示
	fmt.Printf("%sKai%s: %s\n", ColorPurple, ColorReset, content)
}

// ANSI 颜色代码
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorGray   = "\033[90m" // 灰色
	ColorBold   = "\033[1m"
)

// 角色颜色映射
var roleColors = map[string]string{
	"product":   ColorGreen,   // 产品 - 绿色
	"developer": ColorBlue,    // 开发 - 蓝色
	"tester":    ColorYellow,  // 测试 - 黄色
	"boss":      ColorPurple,  // Boss - 紫色
}

// 名字到角色的映射
var nameToRole = map[string]string{
	"Sarah": "product",
	"Alex":  "developer",
	"Mia":   "tester",
}

// colorizeMeetingReply 给会议回复添加颜色
func colorizeMeetingReply(content string) string {
	// 提取名字和角色
	// 格式: [meetingID] **Name**: content 或 **Name**: content
	name := extractNameFromReply(content)
	role := ""
	if r, ok := nameToRole[name]; ok {
		role = r
	}
	// 特殊处理 boss
	if name == "boss" {
		role = "boss"
	}

	// 获取颜色
	color := ColorWhite
	if c, ok := roleColors[role]; ok {
		color = c
	}

	// 去掉 [meetingID] 前缀
	cleanContent := content
	if idx := strings.Index(content, "] **"); idx != -1 {
		cleanContent = content[idx+2:] // 去掉 "] "
	}

	// 替换 **Name** 为带颜色的版本
	if name != "" {
		oldName := fmt.Sprintf("**%s**", name)
		newName := fmt.Sprintf("%s%s%s%s", ColorBold, color, name, ColorReset)
		cleanContent = strings.Replace(cleanContent, oldName, newName, 1)
	}

	return cleanContent
}

// colorizeStaffMessage 给普通 Staff 消息添加颜色
func colorizeStaffMessage(staffID, content string) string {
	// 从 staffID 推断角色 (如 "developer-xxx")
	role := ""
	if strings.HasPrefix(staffID, "product") {
		role = "product"
	} else if strings.HasPrefix(staffID, "developer") {
		role = "developer"
	} else if strings.HasPrefix(staffID, "tester") {
		role = "tester"
	}

	color := ColorWhite
	if c, ok := roleColors[role]; ok {
		color = c
	}

	return fmt.Sprintf("%s[%s]%s %s", color, staffID[:8], ColorReset, content)
}

// extractNameFromReply 从回复中提取名字
func extractNameFromReply(content string) string {
	// 匹配 **Name**: 格式
	re := regexp.MustCompile(`\*\*([^*]+)\*\*:`)
	matches := re.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// cleanMeetingReply 清理会议回复格式（已废弃，使用 colorizeMeetingReply）
func cleanMeetingReply(content string) string {
	return colorizeMeetingReply(content)
}

// extractMentions 提取 @ 提及并转换为 role
func extractMentions(content string) []string {
	var mentions []string
	words := strings.Fields(content)
	for _, w := range words {
		if strings.HasPrefix(w, "@") {
			name := strings.TrimPrefix(w, "@")
			// 尝试转换为 role
			if role, ok := nameToRole[name]; ok {
				mentions = append(mentions, role)
			} else {
				// 直接使用（可能是 role 名称）
				mentions = append(mentions, name)
			}
		}
	}
	return mentions
}
