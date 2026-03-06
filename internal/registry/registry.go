package registry

import (
	"cyberteam/internal/protocol"
	"fmt"
	"sync"
)

// Registry 员工花名册 - 老板手里的那张表
type Registry struct {
	mu      sync.RWMutex
	workers map[string]*protocol.WorkerProfile // id -> profile
}

func New() *Registry {
	return &Registry{
		workers: make(map[string]*protocol.WorkerProfile),
	}
}

// Register 新员工入职
func (r *Registry) Register(profile *protocol.WorkerProfile) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.workers[profile.ID]; exists {
		return fmt.Errorf("worker %s already registered", profile.ID)
	}

	r.workers[profile.ID] = profile
	return nil
}

// Unregister 员工离职
func (r *Registry) Unregister(workerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.workers, workerID)
}

// Get 查询员工档案
func (r *Registry) Get(workerID string) (*protocol.WorkerProfile, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.workers[workerID]
	return w, ok
}

// UpdateStatus 更新员工状态
func (r *Registry) UpdateStatus(workerID string, status protocol.WorkerStatus, load int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	w, ok := r.workers[workerID]
	if !ok {
		return fmt.Errorf("worker %s not found", workerID)
	}
	w.Status = status
	w.Load = load
	return nil
}

// Match 根据任务匹配合适的员工
// 策略: 1) 能力匹配 2) 空闲优先 3) 负载低优先
func (r *Registry) Match(taskType string) (*protocol.WorkerProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var candidates []*protocol.WorkerProfile

	for _, w := range r.workers {
		// 检查是否具备该能力
		for _, cap := range w.Capabilities {
			if cap.Name == taskType {
				candidates = append(candidates, w)
				break
			}
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no worker can handle task type: %s", taskType)
	}

	// 选择最佳候选人: 空闲 > 忙碌但负载低 > 其他
	var best *protocol.WorkerProfile
	bestScore := -1

	for _, c := range candidates {
		score := 0
		if c.Status == protocol.StatusIdle {
			score += 100
		}
		score += (100 - c.Load) // 负载越低分越高

		if score > bestScore {
			bestScore = score
			best = c
		}
	}

	return best, nil
}

// ListByRole 按角色列出员工
func (r *Registry) ListByRole(role string) []*protocol.WorkerProfile {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*protocol.WorkerProfile
	for _, w := range r.workers {
		if w.Role == role {
			result = append(result, w)
		}
	}
	return result
}

// ListAll 列出所有员工
func (r *Registry) ListAll() []*protocol.WorkerProfile {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*protocol.WorkerProfile, 0, len(r.workers))
	for _, w := range r.workers {
		result = append(result, w)
	}
	return result
}
