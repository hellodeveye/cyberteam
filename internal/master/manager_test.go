package master

import (
	"sort"
	"testing"

	"cyberteam/internal/protocol"
	"cyberteam/internal/registry"
	"cyberteam/internal/workflow"
)

// newTestManager 创建一个仅初始化 registry 的 Manager，用于单元测试（不启动任何进程）
func newTestManager() *Manager {
	wf := workflow.CreateDevWorkflow()
	engine := workflow.NewEngine(wf)
	return &Manager{
		engine:   engine,
		registry: registry.New(),
		staffs:   make(map[string]*StaffProcess),
	}
}

func registerWorker(r *registry.Registry, id, name, role string, status protocol.WorkerStatus) {
	_ = r.Register(&protocol.WorkerProfile{
		ID:     id,
		Name:   name,
		Role:   role,
		Status: status,
	})
}

// TestGetNameToRoleMap_Empty 空 Registry 返回空 map
func TestGetNameToRoleMap_Empty(t *testing.T) {
	m := newTestManager()
	result := m.GetNameToRoleMap()
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

// TestGetNameToRoleMap_WithStaffs 已注册的 staff 能被正确映射
func TestGetNameToRoleMap_WithStaffs(t *testing.T) {
	m := newTestManager()
	registerWorker(m.registry, "product-1", "Sarah", "product", protocol.StatusIdle)
	registerWorker(m.registry, "developer-1", "Alex", "developer", protocol.StatusIdle)
	registerWorker(m.registry, "tester-1", "Mia", "tester", protocol.StatusOffline)

	got := m.GetNameToRoleMap()

	cases := map[string]string{
		"Sarah": "product",
		"Alex":  "developer",
		"Mia":   "tester",
	}
	for name, wantRole := range cases {
		if got[name] != wantRole {
			t.Errorf("GetNameToRoleMap()[%q] = %q, want %q", name, got[name], wantRole)
		}
	}
	if len(got) != 3 {
		t.Errorf("expected 3 entries, got %d", len(got))
	}
}

// TestGetNameToRoleMap_NoNameSkipped 没有名字的 staff 不进入映射
func TestGetNameToRoleMap_NoNameSkipped(t *testing.T) {
	m := newTestManager()
	// 注册一个没有名字的 worker
	_ = m.registry.Register(&protocol.WorkerProfile{
		ID:   "product-2",
		Name: "", // 空名字
		Role: "product",
	})
	got := m.GetNameToRoleMap()
	if len(got) != 0 {
		t.Errorf("expected empty map when name is empty, got %v", got)
	}
}

// TestGetOnlineStaffNames_Empty 空 Registry 返回空列表
func TestGetOnlineStaffNames_Empty(t *testing.T) {
	m := newTestManager()
	names := m.GetOnlineStaffNames()
	if len(names) != 0 {
		t.Errorf("expected empty list, got %v", names)
	}
}

// TestGetOnlineStaffNames_OnlyOnline 只返回非 offline 的 staff
func TestGetOnlineStaffNames_OnlyOnline(t *testing.T) {
	m := newTestManager()
	registerWorker(m.registry, "product-1", "Sarah", "product", protocol.StatusIdle)
	registerWorker(m.registry, "developer-1", "Alex", "developer", protocol.StatusBusy)
	registerWorker(m.registry, "tester-1", "Mia", "tester", protocol.StatusOffline) // offline，不应出现

	names := m.GetOnlineStaffNames()
	sort.Strings(names)

	want := []string{"Alex", "Sarah"}
	if len(names) != len(want) {
		t.Fatalf("expected %v, got %v", want, names)
	}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, n, want[i])
		}
	}
}

// TestGetOnlineStaffNames_AllOffline 全部 offline 时返回空列表
func TestGetOnlineStaffNames_AllOffline(t *testing.T) {
	m := newTestManager()
	registerWorker(m.registry, "product-1", "Sarah", "product", protocol.StatusOffline)
	registerWorker(m.registry, "tester-1", "Mia", "tester", protocol.StatusOffline)

	names := m.GetOnlineStaffNames()
	if len(names) != 0 {
		t.Errorf("expected empty list when all offline, got %v", names)
	}
}
