package main

import (
	"fmt"
	"strings"

	"cyberteam/internal/session"
)

// handleChat 处理私聊命令
func handleChat(sess *session.Session, args []string) {
	if len(args) < 1 {
		printChatHelp()
		return
	}

	name := args[0]

	// 从 Registry 动态查询 role（不再硬编码）
	role, ok := roleForName(name)
	if !ok {
		fmt.Printf("❌ 未知员工: %s\n", name)
		online := getOnlineStaffNames()
		if len(online) > 0 {
			fmt.Printf("   可用: %s\n", strings.Join(online, ", "))
		}
		return
	}

	// 检查员工是否在线
	if !gBoss.IsStaffOnline(role) {
		fmt.Printf("❌ %s 当前不在线\n", name)
		return
	}

	// 进入私聊模式
	sess.SetPrivateChat(name)

	// 注册私聊消息回调
	gBoss.SetPrivateMessageCallback(func(staffName, content string) {
		// 只显示当前私聊对象的消息
		pc := sess.GetPrivateChat()
		if pc != nil && pc.With == staffName {
			color := getSenderColor(staffName)
			fmt.Printf("\n%s%s%s: %s\n", color, staffName, ColorReset, content)
			sess.AddPrivateChatMessage(staffName, content)
			fmt.Print(sess.GetPrompt())
		}
	})

	fmt.Printf("\n💬 开始和 %s 私聊\n", name)
	fmt.Println("   直接输入消息发送")
	fmt.Println("   .. 退出私聊")
}

// handlePrivateMessage 处理私聊消息
func handlePrivateMessage(sess *session.Session, content string) {
	pc := sess.GetPrivateChat()
	if pc == nil {
		return
	}

	// 从 Registry 动态获取 role
	role, ok := roleForName(pc.With)
	if !ok {
		fmt.Println("❌ 私聊对象错误")
		return
	}

	bossName := getBossName()
	sess.AddPrivateChatMessage(bossName, content)
	history := sess.GetPrivateChatHistory()

	go gBoss.SendPrivateMessage(role, "boss", content, history)

	fmt.Printf("%s%s%s: %s\n", ColorPurple, bossName, ColorReset, content)
}

// printChatHelp 动态生成 chat 帮助文本
func printChatHelp() {
	fmt.Println("❌ 用法: chat <name>")
	names := getOnlineStaffNames()
	if len(names) > 0 {
		for _, n := range names {
			fmt.Printf("   chat %-10s - 和 %s 私聊\n", n, n)
		}
	} else {
		fmt.Println("   (暂无在线员工)")
	}
	fmt.Println("   ..             - 退出私聊")
}
