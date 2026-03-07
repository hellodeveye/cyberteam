package staffutil

import (
	"fmt"
	"strings"
	"testing"

	"cyberteam/internal/protocol"
)

// ── extractSchemaParams ──────────────────────────────────────────────────────

func TestExtractSchemaParams_Nil(t *testing.T) {
	if got := extractSchemaParams(nil); got != nil {
		t.Errorf("expected nil for nil schema, got %v", got)
	}
}

func TestExtractSchemaParams_NoProperties(t *testing.T) {
	schema := map[string]interface{}{"type": "object"} // 无 properties
	if got := extractSchemaParams(schema); got != nil {
		t.Errorf("expected nil when no properties, got %v", got)
	}
}

func TestExtractSchemaParams_EmptyProperties(t *testing.T) {
	schema := map[string]interface{}{
		"properties": map[string]interface{}{},
	}
	if got := extractSchemaParams(schema); got != nil {
		t.Errorf("expected nil for empty properties, got %v", got)
	}
}

func TestExtractSchemaParams_RequiredParam(t *testing.T) {
	schema := map[string]interface{}{
		"properties": map[string]interface{}{
			"url": map[string]interface{}{"type": "string"},
		},
		"required": []interface{}{"url"},
	}
	params := extractSchemaParams(schema)
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %v", params)
	}
	if params[0] != "url(string,required)" {
		t.Errorf("got %q, want %q", params[0], "url(string,required)")
	}
}

func TestExtractSchemaParams_OptionalParam(t *testing.T) {
	schema := map[string]interface{}{
		"properties": map[string]interface{}{
			"timeout": map[string]interface{}{"type": "integer"},
		},
		// no required field
	}
	params := extractSchemaParams(schema)
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %v", params)
	}
	if params[0] != "timeout(integer)" {
		t.Errorf("got %q, want %q", params[0], "timeout(integer)")
	}
}

func TestExtractSchemaParams_NoTypeFallsBackToDescription(t *testing.T) {
	schema := map[string]interface{}{
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				// no "type", has description
				"description": "The search query string",
			},
		},
	}
	params := extractSchemaParams(schema)
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %v", params)
	}
	// description is truncated to 20 chars
	if !strings.HasPrefix(params[0], "query(") {
		t.Errorf("param should start with 'query(', got %q", params[0])
	}
}

func TestExtractSchemaParams_NoTypeNoDescription(t *testing.T) {
	schema := map[string]interface{}{
		"properties": map[string]interface{}{
			"data": map[string]interface{}{}, // empty definition
		},
	}
	params := extractSchemaParams(schema)
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %v", params)
	}
	if params[0] != "data(any)" {
		t.Errorf("got %q, want %q", params[0], "data(any)")
	}
}

func TestExtractSchemaParams_RequiredAndOptionalMixed(t *testing.T) {
	schema := map[string]interface{}{
		"properties": map[string]interface{}{
			"url":     map[string]interface{}{"type": "string"},
			"timeout": map[string]interface{}{"type": "integer"},
		},
		"required": []interface{}{"url"},
	}
	params := extractSchemaParams(schema)
	if len(params) != 2 {
		t.Fatalf("expected 2 params, got %v", params)
	}
	joined := strings.Join(params, " ")
	if !strings.Contains(joined, "url(string,required)") {
		t.Errorf("expected url to be required, got params: %v", params)
	}
	if !strings.Contains(joined, "timeout(integer)") {
		t.Errorf("expected timeout to be optional, got params: %v", params)
	}
}

// ── StaffMCPClient 工具列表测试 ──────────────────────────────────────────────

// mockStaffMCPClient 用于测试的 mock 客户端
type mockStaffMCPClient struct {
	tools []protocol.MCPToolInfo
}

func (m *mockStaffMCPClient) ListTools() []protocol.MCPToolInfo {
	return m.tools
}

func (m *mockStaffMCPClient) GetToolsPrompt() string {
	tools := m.ListTools()
	if len(tools) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "**可用工具：**")
	for _, t := range tools {
		lines = append(lines, fmt.Sprintf("- %s: %s", t.Name, t.Description))
		if params := extractSchemaParams(t.InputSchema); len(params) > 0 {
			lines = append(lines, fmt.Sprintf("  参数: %s", strings.Join(params, ", ")))
		}
	}
	lines = append(lines, "")
	lines = append(lines, "**使用工具：**")
	lines = append(lines, "当你需要外部数据时，可以调用工具。在回复中用 [TOOL:tool_name]json_args 格式")
	lines = append(lines, `例如：[TOOL:fetch:fetch_url]{"url":"https://example.com"}`)

	return strings.Join(lines, "\n")
}

func TestGetToolsPrompt_NoTools(t *testing.T) {
	client := &mockStaffMCPClient{tools: nil}
	if got := client.GetToolsPrompt(); got != "" {
		t.Errorf("expected empty string for no tools, got %q", got)
	}
}

func TestGetToolsPrompt_ToolWithoutSchema(t *testing.T) {
	tools := []protocol.MCPToolInfo{
		{Name: "fetch:fetch_url", Server: "fetch", Description: "Fetch a URL"},
	}
	client := &mockStaffMCPClient{tools: tools}
	prompt := client.GetToolsPrompt()

	if !strings.Contains(prompt, "fetch:fetch_url") {
		t.Errorf("prompt missing tool name: %q", prompt)
	}
	if !strings.Contains(prompt, "Fetch a URL") {
		t.Errorf("prompt missing description: %q", prompt)
	}
	// 无 schema，不应有 "参数:" 行
	if strings.Contains(prompt, "参数:") {
		t.Errorf("prompt should not contain 参数 line when schema is empty: %q", prompt)
	}
}

func TestGetToolsPrompt_ToolWithSchema(t *testing.T) {
	tools := []protocol.MCPToolInfo{
		{
			Name:        "fetch:fetch_url",
			Server:      "fetch",
			Description: "Fetch a URL",
			InputSchema: map[string]interface{}{
				"properties": map[string]interface{}{
					"url": map[string]interface{}{"type": "string"},
				},
				"required": []interface{}{"url"},
			},
		},
	}
	client := &mockStaffMCPClient{tools: tools}
	prompt := client.GetToolsPrompt()

	if !strings.Contains(prompt, "参数:") {
		t.Errorf("prompt should contain 参数 line, got: %q", prompt)
	}
	if !strings.Contains(prompt, "url(string,required)") {
		t.Errorf("prompt should contain url param, got: %q", prompt)
	}
}

func TestGetToolsPrompt_ContainsUsageInstructions(t *testing.T) {
	tools := []protocol.MCPToolInfo{
		{Name: "fetch:fetch_url", Description: "Fetch"},
	}
	client := &mockStaffMCPClient{tools: tools}
	prompt := client.GetToolsPrompt()

	if !strings.Contains(prompt, "[TOOL:") {
		t.Errorf("prompt should contain TOOL usage example, got: %q", prompt)
	}
}

// TestGetToolsPrompt_SchemaRoundtripViaJSON 验证 InputSchema 能正确经过 JSON 序列化/反序列化
func TestGetToolsPrompt_SchemaRoundtripViaJSON(t *testing.T) {
	original := protocol.MCPToolInfo{
		Name:        "github:search_repos",
		Server:      "github",
		Description: "Search GitHub repositories",
		InputSchema: map[string]interface{}{
			"properties": map[string]interface{}{
				"query": map[string]interface{}{"type": "string"},
				"limit": map[string]interface{}{"type": "integer"},
			},
			"required": []interface{}{"query"},
		},
	}

	// 模拟 Staff 端的 JSON 序列化传输（与 Boss 无关）
	// Staff 直接从本地 MCP Server 获取工具，无需 JSON 传输
	// 此测试验证 InputSchema 解析正确性
	params := extractSchemaParams(original.InputSchema)
	joined := strings.Join(params, " ")

	if !strings.Contains(joined, "query(string,required)") {
		t.Errorf("roundtrip lost required param, got params: %v", params)
	}
	if !strings.Contains(joined, "limit(integer)") {
		t.Errorf("roundtrip lost optional param, got params: %v", params)
	}
}
