package mcp

import (
	"encoding/json"
	"testing"

	"cyberteam/internal/protocol"
)

// newTestManager 构造一个不依赖外部进程的 Manager，直接注入 ServerInstance
func newTestManager(serverName string, tools []Tool, roles []string) *Manager {
	inst := &ServerInstance{
		Name:    serverName,
		Tools:   tools,
		pending: make(map[string]chan *JSONRPCResponse),
		ready:   true,
	}
	return &Manager{
		config: &Config{
			Servers: map[string]Server{
				serverName: {
					Enabled: true,
					ACL:     ACL{Roles: roles},
				},
			},
		},
		servers: map[string]*ServerInstance{serverName: inst},
		logger:  func(string, ...interface{}) {},
	}
}

// ── ListTools: InputSchema 传递 ──────────────────────────────────────────────

func TestListTools_InputSchemaForwarded(t *testing.T) {
	schema := map[string]interface{}{
		"properties": map[string]interface{}{
			"url": map[string]interface{}{"type": "string"},
		},
		"required": []interface{}{"url"},
	}
	m := newTestManager("fetch", []Tool{
		{Name: "fetch_url", Description: "Fetch a URL", InputSchema: schema},
	}, []string{"developer"})

	tools := m.ListTools("developer")
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].InputSchema == nil {
		t.Fatal("InputSchema should be forwarded, got nil")
	}
	props, ok := tools[0].InputSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("InputSchema.properties missing or wrong type: %v", tools[0].InputSchema)
	}
	if _, ok := props["url"]; !ok {
		t.Error("InputSchema.properties should contain 'url'")
	}
}

func TestListTools_NilInputSchemaForwardedAsNil(t *testing.T) {
	m := newTestManager("fetch", []Tool{
		{Name: "fetch_url", Description: "Fetch a URL", InputSchema: nil},
	}, []string{"developer"})

	tools := m.ListTools("developer")
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].InputSchema != nil {
		t.Errorf("expected nil InputSchema, got %v", tools[0].InputSchema)
	}
}

// ── ListTools: 工具名格式 ────────────────────────────────────────────────────

func TestListTools_FullNameFormat(t *testing.T) {
	m := newTestManager("github", []Tool{
		{Name: "search_repos", Description: "Search repos"},
	}, []string{"developer"})

	tools := m.ListTools("developer")
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "github:search_repos" {
		t.Errorf("Name = %q, want %q", tools[0].Name, "github:search_repos")
	}
	if tools[0].Server != "github" {
		t.Errorf("Server = %q, want %q", tools[0].Server, "github")
	}
}

// ── ListTools: 角色权限过滤 ──────────────────────────────────────────────────

func TestListTools_RoleNotAllowed(t *testing.T) {
	m := newTestManager("github", []Tool{
		{Name: "search_repos", Description: "Search repos"},
	}, []string{"developer"})

	if tools := m.ListTools("product"); len(tools) != 0 {
		t.Errorf("expected no tools for product role, got %v", tools)
	}
}

func TestListTools_EmptyRole(t *testing.T) {
	m := newTestManager("fetch", []Tool{
		{Name: "fetch_url", Description: "Fetch a URL"},
	}, []string{"developer"})

	if tools := m.ListTools(""); len(tools) != 0 {
		t.Errorf("expected no tools for empty role, got %v", tools)
	}
}

func TestListTools_MultipleRoles(t *testing.T) {
	m := newTestManager("fetch", []Tool{
		{Name: "fetch_url", Description: "Fetch a URL"},
	}, []string{"developer", "product", "tester"})

	for _, role := range []string{"developer", "product", "tester"} {
		if tools := m.ListTools(role); len(tools) != 1 {
			t.Errorf("role %q: expected 1 tool, got %d", role, len(tools))
		}
	}
}

// ── ListTools: DeniedTools ACL ───────────────────────────────────────────────

func TestListTools_DeniedToolFiltered(t *testing.T) {
	inst := &ServerInstance{
		Name: "github",
		Tools: []Tool{
			{Name: "search_repos"},
			{Name: "delete_repo"},
		},
		pending: make(map[string]chan *JSONRPCResponse),
		ready:   true,
	}
	m := &Manager{
		config: &Config{
			Servers: map[string]Server{
				"github": {
					Enabled: true,
					ACL: ACL{
						Roles:       []string{"developer"},
						DeniedTools: []string{"delete_repo"},
					},
				},
			},
		},
		servers: map[string]*ServerInstance{"github": inst},
		logger:  func(string, ...interface{}) {},
	}

	tools := m.ListTools("developer")
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool after deny filter, got %d: %v", len(tools), tools)
	}
	if tools[0].Name != "github:search_repos" {
		t.Errorf("wrong tool returned: %q", tools[0].Name)
	}
}

// ── ListTools: MCPToolInfo 字段完整性 ────────────────────────────────────────

func TestListTools_AllFieldsPopulated(t *testing.T) {
	schema := map[string]interface{}{"type": "object"}
	m := newTestManager("fetch", []Tool{
		{Name: "fetch_url", Description: "Fetch a URL", InputSchema: schema},
	}, []string{"developer"})

	tools := m.ListTools("developer")
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool := tools[0]
	if tool.Name == "" {
		t.Error("Name should not be empty")
	}
	if tool.Server == "" {
		t.Error("Server should not be empty")
	}
	if tool.Description == "" {
		t.Error("Description should not be empty")
	}
	if tool.InputSchema == nil {
		t.Error("InputSchema should not be nil")
	}
}

// ── MCPToolInfo JSON roundtrip（Boss→Staff 传输路径） ────────────────────────

func TestMCPToolInfo_JSONRoundtrip(t *testing.T) {
	original := protocol.MCPToolInfo{
		Name:        "fetch:fetch_url",
		Server:      "fetch",
		Description: "Fetch a URL",
		InputSchema: map[string]interface{}{
			"properties": map[string]interface{}{
				"url": map[string]interface{}{"type": "string"},
			},
			"required": []interface{}{"url"},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded protocol.MCPToolInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.InputSchema == nil {
		t.Fatal("InputSchema lost during JSON roundtrip")
	}
	props, ok := decoded.InputSchema["properties"].(map[string]interface{})
	if !ok || props["url"] == nil {
		t.Errorf("InputSchema.properties.url lost, got: %v", decoded.InputSchema)
	}
}
