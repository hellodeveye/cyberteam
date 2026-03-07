package staffutil

// GenericMeetingHandler is a reusable meeting handler for all staff types.
type GenericMeetingHandler struct {
	Participant *MeetingParticipant
}

func (h *GenericMeetingHandler) HandleMeetingMessage(meetingID string, from string, content string, mentioned bool, transcript string, team []map[string]string) string {
	// 将团队成员转换为 TeamMember 格式
	if len(team) > 0 {
		members := make([]TeamMember, len(team))
		for i, m := range team {
			members[i] = TeamMember{
				Name: m["name"],
				Role: m["role"],
			}
		}
		h.Participant.SetTeamMembers(members)
	}
	return h.Participant.GenerateReply(meetingID, "", transcript, from, content, mentioned)
}

// GenericPrivateHandler is a reusable private chat handler for all staff types.
type GenericPrivateHandler struct {
	Participant *MeetingParticipant
}

func (h *GenericPrivateHandler) HandlePrivateMessage(from string, content string, history string) string {
	return h.Participant.GenerateReply("", "私聊", history, from, content, true)
}
