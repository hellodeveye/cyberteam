package staffutil

// GenericMeetingHandler is a reusable meeting handler for all staff types.
type GenericMeetingHandler struct {
	Participant *MeetingParticipant
}

func (h *GenericMeetingHandler) HandleMeetingMessage(meetingID string, from string, content string, mentioned bool, transcript string) string {
	return h.Participant.GenerateReply(meetingID, "", transcript, from, content, mentioned)
}

// GenericPrivateHandler is a reusable private chat handler for all staff types.
type GenericPrivateHandler struct {
	Participant *MeetingParticipant
}

func (h *GenericPrivateHandler) HandlePrivateMessage(from string, content string, history string) string {
	return h.Participant.GenerateReply("", "私聊", history, from, content, true)
}
