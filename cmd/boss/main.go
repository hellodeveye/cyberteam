package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cyberteam/internal/master"
	"cyberteam/internal/mcp"
	"cyberteam/internal/meeting"
	"cyberteam/internal/profile"
	"cyberteam/internal/session"
	"cyberteam/internal/storage"
	"cyberteam/internal/workflow"
	"cyberteam/internal/workspace"

	"github.com/chzyer/readline"
)

// 全局变量（简化命令处理）
var gWsManager *workspace.Manager
var gBoss *master.Manager
var gMeetingRoom *meeting.Room
var gMCPManager *mcp.Manager

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
	msgQueue := session.NewMessageQueue()

	// 会话状态
	sess := session.NewSession()

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

	// 召集员工 - 自发现机制
	staffDir := filepath.Join(rootDir, "cmd/staff")

	fmt.Println("\n🎯 正在召集团队...")
	discovered, err := boss.DiscoverStaffs(staffDir)
	if err != nil {
		fmt.Printf("❌ 召集团队失败: %v\n", err)
	}
	if len(discovered) == 0 {
		fmt.Println("⚠️ 未发现任何员工，请检查 cmd/staff/ 目录")
	}

	fmt.Println("\n✅ 全员到齐，准备开工！")
	time.Sleep(500 * time.Millisecond)

	// 初始化会议室
	meetingDir := filepath.Join(rootDir, "meetings")
	gMeetingRoom = meeting.NewRoom(meetingDir)
	fmt.Println("✅ 会议室就绪！")

	// 初始化 MCP 管理器（异步启动，避免阻塞）
	mcpConfigPath := filepath.Join(rootDir, "config", "mcp.yaml")
	mcpManager, err := mcp.NewManager(mcpConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ MCP 配置加载失败: %v\n", err)
	} else {
		boss.SetMCPManager(mcpManager)
		gMCPManager = mcpManager

		// 异步启动 MCP Server，避免阻塞主线程
		go func() {
			if err := mcpManager.StartAll(); err != nil {
				fmt.Fprintf(os.Stderr, "⚠️ MCP 启动失败: %v\n", err)
			}
		}()
	}

	// 设置事件监听（需要在 boss 创建后）
	setupEventListeners(engine, msgQueue, sess, wsManager, boss)

	// 恢复未完成的任务
	resumeTasks(engine, boss)

	// 显示帮助
	printHelp()

	// 交互式命令行（使用 readline 支持中文和 Ctrl+C）
	rl, err := readline.New(sess.GetPrompt())
	if err != nil {
		// 降级到标准输入（使用 bufio 简单回退）
		fmt.Println("⚠️ 读取终端失败，使用标准输入模式")
		fmt.Println("提示: 安装 readline 可获得更好的输入体验")
		fmt.Print(sess.GetPrompt())

		// 简单标准输入回退
		var input string
		for {
			if _, err := fmt.Scanln(&input); err != nil {
				return
			}
			processInput(strings.TrimSpace(input), engine, boss, sess)
			fmt.Print(sess.GetPrompt())
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

		processInput(line, engine, boss, sess)
		// 更新提示符（会议状态可能改变）
		rl.SetPrompt(sess.GetPrompt())
	}
}

