package staffutil

import (
	"encoding/json"
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

// ── GetToolsPrompt ───────────────────────────────────────────────────────────

// mockRequest はリスト応答をエミュレートする requestFunc
func mockRequest(tools []protocol.MCPToolInfo) func(protocol.Message) (*protocol.Message, error) {
	return func(msg protocol.Message) (*protocol.Message, error) {
		// ListTools は tools を JSON 経由で受け取る
		toolsData, _ := json.Marshal(tools)
		var raw []interface{}
		json.Unmarshal(toolsData, &raw)

		return &protocol.Message{
			Type: protocol.MsgMCPResult,
			Payload: map[string]interface{}{
				"tools": raw,
			},
		}, nil
	}
}

func TestGetToolsPrompt_NoTools(t *testing.T) {
	client := NewMCPClient(mockRequest(nil))
	if got := client.GetToolsPrompt(); got != "" {
		t.Errorf("expected empty string for no tools, got %q", got)
	}
}

func TestGetToolsPrompt_ToolWithoutSchema(t *testing.T) {
	tools := []protocol.MCPToolInfo{
		{Name: "fetch:fetch_url", Server: "fetch", Description: "Fetch a URL"},
	}
	client := NewMCPClient(mockRequest(tools))
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
	client := NewMCPClient(mockRequest(tools))
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
	client := NewMCPClient(mockRequest(tools))
	prompt := client.GetToolsPrompt()

	if !strings.Contains(prompt, "[TOOL:") {
		t.Errorf("prompt should contain TOOL usage example, got: %q", prompt)
	}
}

// TestGetToolsPrompt_SchemaRoundtripViaJSON 验证 InputSchema 能正确经过 JSON 序列化/反序列化
// 这是集成关键：Boss 通过 JSON 把 MCPToolInfo 传给 Staff
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

	// 模拟 Boss → Staff 的 JSON 序列化传输
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded protocol.MCPToolInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	params := extractSchemaParams(decoded.InputSchema)
	joined := strings.Join(params, " ")

	if !strings.Contains(joined, "query(string,required)") {
		t.Errorf("roundtrip lost required param, got params: %v", params)
	}
	if !strings.Contains(joined, "limit(integer)") {
		t.Errorf("roundtrip lost optional param, got params: %v", params)
	}
}
