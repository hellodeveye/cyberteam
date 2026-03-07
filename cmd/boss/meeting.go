package main

import (
	"fmt"
	"strings"

	"cyberteam/internal/common"
	"cyberteam/internal/master"
	"cyberteam/internal/meeting"
	"cyberteam/internal/session"
	"cyberteam/internal/workflow"
)

// handleMeeting 处理会议命令
func handleMeeting(sess *session.Session, parts []string) {
	if len(parts) < 2 {
		fmt.Println("❌ 用法: meeting <start|list|join|say|ask|end|transcript>")
		return
	}

	subCmd := parts[1]
	args := parts[2:]

	switch subCmd {
	case "start":
		handleMeetingStart(sess, args)
	case "list", "ls":
		handleMeetingList()
	case "join":
		handleMeetingJoin(sess, args)
	case "end":
		handleMeetingEnd(sess)
	case "transcript", "log":
		handleMeetingTranscript(sess)
	default:
		fmt.Printf("❌ 未知会议命令: %s\n", subCmd)
	}
}

func handleMeetingStart(sess *session.Session, args []string) {
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
	sess.SetMeeting(mtg)

	// 注册消息回调（流式显示）
	gMeetingRoom.OnMessage(func(meetingID string, msg meeting.Message) {
		if meetingID != mtg.ID {
			return
		}
		// 不显示 Boss 自己的消息（避免重复）
		if msg.From == "boss" {
			return
		}
		displayMeetingMessage(msg, sess)
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

func handleMeetingJoin(sess *session.Session, args []string) {
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

	sess.SetMeeting(mtg)

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
			displayMeetingMessage(msg, sess)
		}
	}
}

func handleMeetingSay(sess *session.Session, args []string) {
	mtg := sess.GetMeeting()
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

func handleMeetingAsk(sess *session.Session, args []string) {
	mtg := sess.GetMeeting()
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
func handleDirectSay(sess *session.Session, content string) {
	mtg := sess.GetMeeting()
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
func handleDirectMention(sess *session.Session, line string) {
	mtg := sess.GetMeeting()
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

func handleMeetingEnd(sess *session.Session) {
	mtg := sess.GetMeeting()
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
	sess.SetMeeting(nil)
}

func handleMeetingTranscript(sess *session.Session) {
	mtg := sess.GetMeeting()
	if mtg == nil {
		fmt.Println("❌ 当前不在会议中")
		return
	}

	fmt.Printf("\n📜 会议 [%s] 记录\n", mtg.Topic)
	fmt.Println(strings.Repeat("-", 80))

	for _, msg := range mtg.Messages {
		displayMeetingMessage(msg, sess)
	}
}

func displayMeetingMessage(msg meeting.Message, sess *session.Session) {
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

// resumeTasks 恢复未完成的任务
func resumeTasks(engine *workflow.Engine, boss *master.Manager) {
	tasks := engine.GetAllTasks()

	var resumed int
	for _, task := range tasks {
		// 只恢复已分配但未完成的任务
		if task.Status == workflow.StatusAssigned || task.Status == workflow.StatusInProgress {
			role := common.GetRoleForStage(task.Stage)
			if role != "" {
				name := task.Name
				if len(name) > 20 {
					name = name[:20]
				}
				fmt.Printf("🔄 恢复任务: %s [%s] → %s\n", name, common.GetStageName(task.Stage), role)
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
