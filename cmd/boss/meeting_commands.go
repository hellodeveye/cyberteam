package main

// meetingCommandsHelp 返回会议命令帮助
func meetingCommandsHelp() string {
	return `
🗣️ 会议命令:
  meeting start <主题> [--mode free|round]  开始会议
  meeting list                              列出会议
  meeting join <ID>                         加入会议
  say <内容>                                发言
  ask <人> <问题>                           点名提问
  meeting end                               结束会议
`
}