package master

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"cyberteam/internal/protocol"
	"cyberteam/internal/registry"
	"cyberteam/internal/workflow"
	"cyberteam/internal/workspace"
)

// Manager 项目经理
type Manager struct {
	engine      *workflow.Engine
	registry    *registry.Registry
	staffs      map[string]*StaffProcess
	mu          sync.RWMutex
	msgCallback func(staffID, msgType, content string)

	// 任务日志存储
	taskLogs map[string][]LogEntry // taskID -> logs
	logsMu   sync.RWMutex

	// 心跳跟踪
	lastHeartbeat map[string]time.Time // staffID -> 最后心跳时间
	heartbeatMu   sync.RWMutex
}

// LogEntry 日志条目
type LogEntry struct {
	Time    time.Time `json:"time"`
	StaffID string    `json:"staff_id"`
	Level   string    `json:"level"` // info, error, success
	Message string    `json:"message"`
}

type StaffProcess struct {
	Profile *protocol.WorkerProfile
	Cmd     *exec.Cmd
	Stdin   io.WriteCloser
	Stdout  io.ReadCloser
	Role    string
}

// NewManager 创建项目经理
func NewManager(engine *workflow.Engine) *Manager {
	m := &Manager{
		engine:        engine,
		registry:      registry.New(),
		staffs:        make(map[string]*StaffProcess),
		taskLogs:      make(map[string][]LogEntry),
		lastHeartbeat: make(map[string]time.Time),
	}

	// 启动心跳超时检测
	go m.heartbeatChecker()

	return m
}

// heartbeatChecker 定期检查员工心跳超时
func (m *Manager) heartbeatChecker() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		m.heartbeatMu.RLock()
		now := time.Now()
		for staffID, lastTime := range m.lastHeartbeat {
			if now.Sub(lastTime) > 90*time.Second {
				// 超时，标记为离线
				m.mu.Lock()
				if proc, ok := m.staffs[staffID]; ok && proc.Profile != nil {
					proc.Profile.Status = protocol.StatusOffline
				}
				m.mu.Unlock()
				m.registry.UpdateStatus(staffID, protocol.StatusOffline, 0)
				fmt.Fprintf(os.Stderr, "[Boss] 员工 %s 心跳超时，已标记为离线\n", staffID[:8])
			}
		}
		m.heartbeatMu.RUnlock()
	}
}

// AddTaskLog 添加任务日志
func (m *Manager) AddTaskLog(taskID, staffID, level, message string) {
	m.logsMu.Lock()
	defer m.logsMu.Unlock()

	m.taskLogs[taskID] = append(m.taskLogs[taskID], LogEntry{
		Time:    time.Now(),
		StaffID: staffID,
		Level:   level,
		Message: message,
	})
}

// GetTaskLogs 获取任务日志
func (m *Manager) GetTaskLogs(taskID string, limit int) []LogEntry {
	m.logsMu.RLock()
	defer m.logsMu.RUnlock()

	logs := m.taskLogs[taskID]
	if limit <= 0 || limit > len(logs) {
		limit = len(logs)
	}

	start := len(logs) - limit
	if start < 0 {
		start = 0
	}

	return logs[start:]
}

// GetTaskLogCount 获取任务日志数量
func (m *Manager) GetTaskLogCount(taskID string) int {
	m.logsMu.RLock()
	defer m.logsMu.RUnlock()
	return len(m.taskLogs[taskID])
}

// SetMessageCallback 设置消息回调
func (m *Manager) SetMessageCallback(cb func(staffID, msgType, content string)) {
	m.msgCallback = cb
}

// HireStaff 招聘员工
func (m *Manager) HireStaff(role, name, binaryPath string) (*protocol.WorkerProfile, error) {
	staffID := fmt.Sprintf("%s-%d", role, time.Now().UnixNano())

	cmd := exec.Command(binaryPath, "--id", staffID, "--name", name)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start staff: %w", err)
	}

	proc := &StaffProcess{
		Cmd:    cmd,
		Stdin:  stdin,
		Stdout: stdout,
		Role:   role,
	}

	m.mu.Lock()
	m.staffs[staffID] = proc
	m.mu.Unlock()

	go m.listenStaff(staffID, stdout)

	// 等待注册
	time.Sleep(1 * time.Second)

	if proc.Profile != nil {
		return proc.Profile, nil
	}

	return nil, fmt.Errorf("staff did not register in time")
}

// listenStaff 监听员工消息
func (m *Manager) listenStaff(staffID string, stdout io.ReadCloser) {
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg protocol.Message
		if err := json.Unmarshal(line, &msg); err != nil {
			fmt.Fprintf(os.Stderr, "[Boss] parse error: %v\n", err)
			continue
		}

		m.handleMessage(staffID, msg)
	}

	m.mu.Lock()
	if proc, ok := m.staffs[staffID]; ok {
		proc.Profile.Status = protocol.StatusOffline
	}
	m.mu.Unlock()
	m.registry.UpdateStatus(staffID, protocol.StatusOffline, 0)
	fmt.Fprintf(os.Stderr, "[Boss] 员工 %s 已离线\n", staffID)
}

// handleMessage 处理员工消息
func (m *Manager) handleMessage(staffID string, msg protocol.Message) {
	switch msg.Type {
	case protocol.MsgRegister:
		if profileData, ok := msg.Payload["profile"]; ok {
			data, _ := json.Marshal(profileData)
			var profile protocol.WorkerProfile
			if err := json.Unmarshal(data, &profile); err != nil {
				return
			}

			m.mu.Lock()
			if proc, ok := m.staffs[staffID]; ok {
				proc.Profile = &profile
			}
			m.mu.Unlock()

			m.registry.Register(&profile)
		}

	case protocol.MsgHeartbeat:
		status, _ := msg.Payload["status"].(string)
		load := 0
		if l, ok := msg.Payload["load"].(float64); ok {
			load = int(l)
		}
		m.registry.UpdateStatus(staffID, protocol.WorkerStatus(status), load)
		// 更新最后心跳时间
		m.heartbeatMu.Lock()
		m.lastHeartbeat[staffID] = time.Now()
		m.heartbeatMu.Unlock()

	case protocol.MsgAccept:
		m.mu.RLock()
		proc := m.staffs[staffID]
		m.mu.RUnlock()
		if proc != nil && proc.Profile != nil {
			content := fmt.Sprintf("🚀 %s 开始处理任务 %s", proc.Profile.Name, msg.TaskID[:8])
			if m.msgCallback != nil {
				m.msgCallback(staffID, string(msg.Type), content)
			}
		}

	case protocol.MsgProgress:
		if logs, ok := msg.Payload["logs"].([]any); ok && len(logs) > 0 {
			lastLog := logs[len(logs)-1]
			content := fmt.Sprintf("   %s", lastLog)
			// 存储日志
			m.AddTaskLog(msg.TaskID, staffID, "info", fmt.Sprintf("%v", lastLog))
			// 通过回调通知（由消息队列统一显示，避免重复打印）
			if m.msgCallback != nil {
				m.msgCallback(staffID, string(msg.Type), content)
			}
		}

	case protocol.MsgComplete:
		m.handleTaskComplete(staffID, msg.TaskID, msg.Payload)

	case protocol.MsgFailed:
		m.handleTaskFailed(staffID, msg.TaskID, msg.Payload)

	case protocol.MsgMeetingReply:
		// Staff 的会议回复
		meetingID, _ := msg.Payload["meeting_id"].(string)
		content, _ := msg.Payload["content"].(string)
		m.mu.RLock()
		proc := m.staffs[staffID]
		m.mu.RUnlock()
		if proc != nil && proc.Profile != nil {
			if m.msgCallback != nil {
				m.msgCallback(staffID, "meeting_reply", fmt.Sprintf("[%s] **%s**: %s", meetingID, proc.Profile.Name, content))
			}
		}
	}
}

// SendMeetingMessage 发送会议消息给 Staff（Boss 主动调用）
func (m *Manager) SendMeetingMessage(staffRole string, meetingID string, from string, content string, mentioned bool, transcript string) error {
	m.mu.RLock()
	var targetStaff *StaffProcess
	for _, proc := range m.staffs {
		if proc.Profile != nil && proc.Profile.Role == staffRole {
			targetStaff = proc
			break
		}
	}
	m.mu.RUnlock()

	if targetStaff == nil {
		return fmt.Errorf("staff with role %s not found", staffRole)
	}

	msg := protocol.Message{
		Type: protocol.MsgMeetingMsg,
		ID:   generateID(),
		Payload: map[string]any{
			"meeting_id": meetingID,
			"from":       from,
			"content":    content,
			"mentioned":  mentioned,
			"transcript": transcript, // 传递会议历史
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = targetStaff.Stdin.Write(append(data, '\n'))
	return err
}

// BroadcastMeetingMessage 广播会议消息给所有 Staff
func (m *Manager) BroadcastMeetingMessage(meetingID string, from string, content string) error {
	m.mu.RLock()
	staffs := make([]*StaffProcess, 0, len(m.staffs))
	for _, proc := range m.staffs {
		staffs = append(staffs, proc)
	}
	m.mu.RUnlock()

	for _, proc := range staffs {
		if proc.Profile != nil && proc.Profile.Role != "boss" {
			go func(p *StaffProcess) {
				msg := protocol.Message{
					Type: protocol.MsgMeetingMsg,
					ID:   generateID(),
					Payload: map[string]any{
						"meeting_id": meetingID,
						"from":       from,
						"content":    content,
						"mentioned":  false,
					},
				}
				data, _ := json.Marshal(msg)
				p.Stdin.Write(append(data, '\n'))
			}(proc)
		}
	}
	return nil
}

// BroadcastMeetingMessageRandom 随机选择 N 个 Staff 发送会议消息
func (m *Manager) BroadcastMeetingMessageRandom(meetingID string, from string, content string, maxCount int, transcript string) error {
	m.mu.RLock()
	staffs := make([]*StaffProcess, 0, len(m.staffs))
	for _, proc := range m.staffs {
		if proc.Profile != nil && proc.Profile.Role != "boss" {
			staffs = append(staffs, proc)
		}
	}
	m.mu.RUnlock()

	if len(staffs) == 0 {
		return nil
	}

	// 随机选择最多 maxCount 个
	selected := staffs
	if len(staffs) > maxCount {
		// 简单随机：打乱顺序后取前 N 个
		for i := len(staffs) - 1; i > 0; i-- {
			j := time.Now().UnixNano() % int64(i+1)
			staffs[i], staffs[j] = staffs[j], staffs[i]
		}
		selected = staffs[:maxCount]
	}

	for _, proc := range selected {
		go func(p *StaffProcess) {
			// 添加小延迟，模拟真实讨论的节奏感
			time.Sleep(time.Duration(time.Now().UnixNano()%3000+1000) * time.Millisecond)
			msg := protocol.Message{
				Type: protocol.MsgMeetingMsg,
				ID:   generateID(),
				Payload: map[string]any{
					"meeting_id": meetingID,
					"from":       from,
					"content":    content,
					"mentioned":  false,
					"transcript": transcript, // 传递会议历史
				},
			}
			data, err := json.Marshal(msg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[Boss] 消息序列化失败: %v\n", err)
				return
			}
			_, err = p.Stdin.Write(append(data, '\n'))
			if err != nil {
				fmt.Fprintf(os.Stderr, "[Boss] 发送消息给 %s 失败: %v\n", p.Role, err)
			}
		}(proc)
	}
	return nil
}

// handleTaskComplete 处理任务完成
func (m *Manager) handleTaskComplete(staffID, taskID string, payload map[string]any) {
	m.mu.RLock()
	proc := m.staffs[staffID]
	m.mu.RUnlock()

	if proc != nil && proc.Profile != nil {
		msg := fmt.Sprintf("✅ %s 完成任务 %s", proc.Profile.Name, taskID[:8])
		if m.msgCallback != nil {
			m.msgCallback(staffID, "complete", msg)
		}
	}

	// 获取任务输出
	var output interface{}
	var outputs map[string]interface{}
	if resultData, ok := payload["result"]; ok {
		data, _ := json.Marshal(resultData)
		json.Unmarshal(data, &output)
		// 同时保存为 map 用于产物保存
		json.Unmarshal(data, &outputs)
	}

	// 保存产物到工作空间（人类友好格式）
	if outputs != nil {
		go m.saveTaskArtifacts(taskID, outputs)
	}

	// 完成工作流任务（异步，避免阻塞）
	go m.engine.CompleteTask(taskID, output)

	// 更新员工状态
	m.registry.UpdateStatus(staffID, protocol.StatusIdle, 0)
}

// saveTaskArtifacts 保存任务产物到工作空间
func (m *Manager) saveTaskArtifacts(taskID string, taskResult map[string]interface{}) {
	if m.engine == nil {
		return
	}

	task := m.engine.GetTask(taskID)
	if task == nil {
		return
	}

	project := m.engine.GetProject(task.ProjectID)
	if project == nil {
		return
	}

	// 获取阶段编号
	stageNum := 0
	switch task.Stage {
	case workflow.StageRequirement:
		stageNum = 1
	case workflow.StageDesign:
		stageNum = 2
	case workflow.StageReview:
		stageNum = 3
	case workflow.StageDevelop:
		stageNum = 4
	case workflow.StageTest:
		stageNum = 5
	case workflow.StageDeploy:
		stageNum = 6
	}

	if stageNum == 0 || task.WorkspaceDir == "" {
		return
	}

	// 从 TaskResult 中提取 Outputs 字段
	// TaskResult 结构：{ "outputs": {...}, "success": true, ... }
	var outputs map[string]interface{}
	if out, ok := taskResult["outputs"]; ok {
		if outMap, ok := out.(map[string]interface{}); ok {
			outputs = outMap
		}
	}
	if outputs == nil {
		outputs = taskResult // 兼容旧格式
	}

	// 转换 TaskResult.Outputs 为 Artifact
	artifact := workspace.TaskResultToArtifact(outputs, stageNum)

	// 创建临时 workspace manager 保存产物
	wsManager := workspace.NewManager(filepath.Dir(task.WorkspaceDir))

	// 保存产物
	if err := wsManager.SaveArtifact(project.Name, project.ID, stageNum, artifact); err != nil {
		fmt.Fprintf(os.Stderr, "[Boss] 保存产物失败: %v\n", err)
	} else {
		// 通知保存成功
		names := workspace.StageArtifacts[stageNum]
		if names.Document != "" && artifact.Document != "" {
			m.AddTaskLog(taskID, "system", "success", fmt.Sprintf("已保存文档: %s", names.Document))
		}
		for filename := range artifact.CodeFiles {
			m.AddTaskLog(taskID, "system", "success", fmt.Sprintf("已保存代码: %s", filename))
		}
	}
}

// handleTaskFailed 处理任务失败
func (m *Manager) handleTaskFailed(staffID, taskID string, payload map[string]any) {
	errMsg := ""
	// 尝试从 result 对象中获取错误
	if resultData, ok := payload["result"]; ok {
		if result, ok := resultData.(map[string]any); ok {
			if err, ok := result["error"].(string); ok {
				errMsg = err
			}
		}
	}
	// 兼容直接 error 字段
	if errMsg == "" {
		if err, ok := payload["error"].(string); ok {
			errMsg = err
		}
	}
	if errMsg == "" {
		errMsg = "未知错误"
	}
	fmt.Printf("❌ 任务 %s 失败: %s\n", taskID[:8], errMsg)

	// 更新员工状态
	m.registry.UpdateStatus(staffID, protocol.StatusIdle, 0)
}

// autoAssignTask 自动分配任务（当前未使用）
func (m *Manager) autoAssignTask(task *workflow.Task) {
	// 此函数保留供将来使用
}

// AssignWorkflowTask 分配工作流任务给员工
func (m *Manager) AssignWorkflowTask(taskID string) error {
	task := m.engine.GetTask(taskID)
	if task == nil {
		return fmt.Errorf("task not found")
	}

	// 如果没有分配人，自动分配一个
	if task.Assignee == "" {
		role := getRoleForStage(task.Stage)
		staffs := m.registry.ListByRole(role)
		if len(staffs) == 0 {
			return fmt.Errorf("no available staff for role: %s", role)
		}
		// 选择第一个空闲的，或第一个
		var selected *protocol.WorkerProfile
		for _, s := range staffs {
			if s.Status == protocol.StatusIdle {
				selected = s
				break
			}
		}
		if selected == nil {
			selected = staffs[0]
		}

		// 分配任务
		if err := m.engine.AssignTask(task.ID, selected.ID); err != nil {
			return fmt.Errorf("failed to assign task: %w", err)
		}

		// 发送任务给员工
		if err := m.sendTaskToStaff(taskID); err != nil {
			return fmt.Errorf("failed to send task to staff: %w", err)
		}
		return nil
	}

	// 如果已经有 Assignee，直接发送
	return m.sendTaskToStaff(taskID)
}

// ReassignTask 强制重新分配任务给指定角色的员工
func (m *Manager) ReassignTask(taskID string, role string) error {
	task := m.engine.GetTask(taskID)
	if task == nil {
		return fmt.Errorf("task not found")
	}

	// 清空旧 Assignee
	m.engine.ClearTaskAssignee(taskID)

	// 查找可用员工
	staffs := m.registry.ListByRole(role)
	if len(staffs) == 0 {
		return fmt.Errorf("no available staff for role: %s", role)
	}

	// 选择第一个空闲的，或第一个
	var selected *protocol.WorkerProfile
	for _, s := range staffs {
		if s.Status == protocol.StatusIdle {
			selected = s
			break
		}
	}
	if selected == nil {
		selected = staffs[0]
	}

	// 分配任务
	if err := m.engine.AssignTask(task.ID, selected.ID); err != nil {
		return fmt.Errorf("failed to assign task: %w", err)
	}

	// 发送任务给员工
	if err := m.sendTaskToStaff(taskID); err != nil {
		return fmt.Errorf("failed to send task to staff: %w", err)
	}
	return nil
}

// sendTaskToStaff 发送任务给员工进程
func (m *Manager) sendTaskToStaff(taskID string) error {
	task := m.engine.GetTask(taskID)
	if task == nil {
		return fmt.Errorf("task not found")
	}

	if task.Assignee == "" {
		return fmt.Errorf("task has no assignee")
	}

	// 找到对应的员工进程
	m.mu.RLock()
	proc, ok := m.staffs[task.Assignee]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("staff process not found: assignee=%s", task.Assignee)
	}

	// 确定任务类型（根据阶段映射到能力）
	capability := getCapabilityForStage(task.Stage)

	// 构造协议任务
	inputs := make(map[string]any)
	if task.Input != nil {
		if m, ok := task.Input.(map[string]any); ok {
			inputs = m
		}
	}
	protocolTask := protocol.Task{
		ID:           task.ID,
		Type:         capability,
		Title:        task.Name,
		Description:  task.Description,
		Inputs:       inputs,
		Priority:     1,
		WorkspaceDir: task.WorkspaceDir,
	}

	// 发送给员工
	msg := protocol.Message{
		Type:    protocol.MsgAssign,
		ID:      generateID(),
		TaskID:  task.ID,
		Payload: map[string]any{"task": protocolTask},
	}

	data, _ := json.Marshal(msg)
	_, err := fmt.Fprintln(proc.Stdin, string(data))
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	// 更新状态
	m.registry.UpdateStatus(task.Assignee, protocol.StatusBusy, 50)

	return nil
}

// ShowTeam 显示团队状态
func (m *Manager) ShowTeam() {
	staffs := m.registry.ListAll()
	fmt.Println("\n👥 团队状态:")
	for _, s := range staffs {
		icon := getRoleIcon(s.Role)
		statusIcon := "🟢"
		if s.Status == protocol.StatusBusy {
			statusIcon = "🔴"
		} else if s.Status == protocol.StatusOffline {
			statusIcon = "⚫"
		}
		fmt.Printf("  %s %s %s - %s (负载%d%%)\n", icon, statusIcon, s.Name, s.Status, s.Load)
	}
	fmt.Println()
}

// Shutdown 关闭所有员工
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, proc := range m.staffs {
		msg := protocol.Message{Type: protocol.MsgShutdown, ID: protocol.GenerateID()}
		data, _ := json.Marshal(msg)
		fmt.Fprintln(proc.Stdin, string(data))
		fmt.Printf("[Boss] 通知 %s 下班\n", id[:8])

		// 等待进程退出
		go func(cmd *exec.Cmd) {
			cmd.Wait()
		}(proc.Cmd)
	}
}

// getRoleForStage 根据阶段获取角色
func getRoleForStage(stage workflow.Stage) string {
	mapping := map[workflow.Stage]string{
		workflow.StageRequirement: "product",
		workflow.StageDesign:      "developer", // 系统设计由 Developer 处理
		workflow.StageReview:      "product",
		workflow.StageDevelop:     "developer",
		workflow.StageTest:        "tester",
		workflow.StageDeploy:      "developer",
	}
	if role, ok := mapping[stage]; ok {
		return role
	}
	return ""
}

// getCapabilityForStage 根据阶段获取能力
func getCapabilityForStage(stage workflow.Stage) string {
	mapping := map[workflow.Stage]string{
		workflow.StageRequirement: "analyze_requirement",
		workflow.StageDesign:      "design_system", // 系统设计
		workflow.StageReview:      "design_review",
		workflow.StageDevelop:     "implement_feature",
		workflow.StageTest:        "execute_test",
		workflow.StageDeploy:      "deploy_service",
	}
	if cap, ok := mapping[stage]; ok {
		return cap
	}
	return ""
}

func getRoleIcon(role string) string {
	icons := map[string]string{
		"product":   "📝",
		"developer": "💻",
		"tester":    "🧪",
	}
	if icon, ok := icons[role]; ok {
		return icon
	}
	return "👤"
}

func generateID() string {
	return protocol.GenerateID()
}
