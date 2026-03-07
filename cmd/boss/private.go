package main

import (
	"fmt"

	"cyberteam/internal/session"
)

// handleChat 处理私聊命令
func handleChat(sess *session.Session, args []string) {
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
	sess.SetPrivateChat(name)

	// 注册私聊消息回调
	gBoss.SetPrivateMessageCallback(func(staffName, content string) {
		// 只显示当前私聊对象的消息
		pc := sess.GetPrivateChat()
		if pc != nil && pc.With == staffName {
			color := getSenderColor(staffName)
			fmt.Printf("\n%s%s%s: %s\n", color, staffName, ColorReset, content)
			// 添加到历史记录
			sess.AddPrivateChatMessage(staffName, content)
			// 刷新提示符
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

	// 获取对方 role
	role, ok := nameToRole[pc.With]
	if !ok {
		fmt.Println("❌ 私聊对象错误")
		return
	}

	// 添加消息到历史
	sess.AddPrivateChatMessage(bossName, content)

	// 获取历史记录
	history := sess.GetPrivateChatHistory()

	// 发送私聊消息给 Staff（带上历史）
	go gBoss.SendPrivateMessage(role, "boss", content, history)

	// 本地显示
	fmt.Printf("%s%s%s: %s\n", ColorPurple, bossName, ColorReset, content)
}
