package worker

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"cyber-company/internal/protocol"
)

// Handler 任务处理接口 - 每个员工要实现这个
type Handler interface {
	// Handle 处理任务，通过 resultChan 返回进度和最终结果
	Handle(task protocol.Task, resultChan chan<- protocol.TaskResult)
}

// BaseWorker 员工基类
type BaseWorker struct {
	Profile *protocol.WorkerProfile
	Handler Handler

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// NewBaseWorker 创建员工
func NewBaseWorker(profile *protocol.WorkerProfile, handler Handler) *BaseWorker {
	return &BaseWorker{
		Profile: profile,
		Handler: handler,
		stdin:   os.Stdin,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
	}
}

// Run 开始工作 - 监听 stdin 接收任务
func (w *BaseWorker) Run() error {
	// 1. 先注册自己
	if err := w.send(protocol.Message{
		Type:    protocol.MsgRegister,
		ID:      generateID(),
		Payload: map[string]any{"profile": w.Profile},
	}); err != nil {
		return fmt.Errorf("register failed: %w", err)
	}

	// 2. 启动心跳
	go w.heartbeat()

	// 3. 主循环：监听任务
	scanner := bufio.NewScanner(w.stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg protocol.Message
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		if err := w.handleMessage(msg); err != nil {
			w.log("handle error: %v", err)
		}
	}

	return scanner.Err()
}

// handleMessage 处理来自老板的消息
func (w *BaseWorker) handleMessage(msg protocol.Message) error {
	switch msg.Type {
	case protocol.MsgAssign:
		return w.handleAssign(msg)
	case protocol.MsgQueryCap:
		return w.send(protocol.Message{
			Type:    protocol.MsgQueryCap,
			ID:      generateID(),
			Payload: map[string]any{"capabilities": w.Profile.Capabilities},
		})
	case protocol.MsgShutdown:
		os.Exit(0)
	}
	return nil
}

// handleAssign 处理任务分配
func (w *BaseWorker) handleAssign(msg protocol.Message) error {
	taskData, _ := json.Marshal(msg.Payload["task"])

	var task protocol.Task
	if err := json.Unmarshal(taskData, &task); err != nil {
		return w.reportFailed(msg.TaskID, "invalid task format: "+err.Error())
	}

	// 确认接受
	w.send(protocol.Message{
		Type:   protocol.MsgAccept,
		ID:     generateID(),
		TaskID: task.ID,
	})

	// 更新状态为忙碌
	w.Profile.Status = protocol.StatusBusy

	// 异步处理任务
	go func() {
		resultChan := make(chan protocol.TaskResult, 10)

		go w.Handler.Handle(task, resultChan)

		// 收集结果
		var finalResult protocol.TaskResult
		for result := range resultChan {
			if result.TaskID == task.ID {
				finalResult = result
				if result.Success || result.Error != "" {
					break
				}
				// 进度更新
				w.send(protocol.Message{
					Type:   protocol.MsgProgress,
					ID:     generateID(),
					TaskID: task.ID,
					Payload: map[string]any{
						"logs": result.Logs,
					},
				})
			}
		}

		// 报告完成
		msgType := protocol.MsgComplete
		if !finalResult.Success {
			msgType = protocol.MsgFailed
		}
		w.send(protocol.Message{
			Type:    msgType,
			ID:      generateID(),
			TaskID:  task.ID,
			Payload: map[string]any{"result": finalResult},
		})

		// 恢复空闲
		w.Profile.Status = protocol.StatusIdle
		w.Profile.Load = 0
	}()

	return nil
}

// heartbeat 定期心跳
func (w *BaseWorker) heartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		w.send(protocol.Message{
			Type: protocol.MsgHeartbeat,
			ID:   generateID(),
			Payload: map[string]any{
				"status": w.Profile.Status,
				"load":   w.Profile.Load,
			},
		})
	}
}

// send 发送消息到 stdout
func (w *BaseWorker) send(msg protocol.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// 使用 bufio 确保立即刷新
	if f, ok := w.stdout.(*os.File); ok {
		writer := bufio.NewWriter(f)
		_, err = writer.Write(data)
		if err != nil {
			return err
		}
		writer.WriteByte('\n')
		return writer.Flush()
	}

	_, err = fmt.Fprintln(w.stdout, string(data))
	return err
}

// log 输出日志到 stderr（仅用于错误）
func (w *BaseWorker) log(format string, args ...any) {
	fmt.Fprintf(w.stderr, "[%s] %s\n", w.Profile.Name, fmt.Sprintf(format, args...))
}

// reportFailed 报告任务失败
func (w *BaseWorker) reportFailed(taskID, reason string) error {
	return w.send(protocol.Message{
		Type:   protocol.MsgFailed,
		ID:     generateID(),
		TaskID: taskID,
		Payload: map[string]any{
			"error": reason,
		},
	})
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
