package agent

import (
	"encoding/json"
	"fmt"
)

// Tool represents a registered tool
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Executor    string // "mcp" or "bash"
}

// ToolRegistry manages tool registration and execution
type ToolRegistry struct {
	Tools     map[string]Tool
	Executors map[string]ToolExecutor
}

// NewToolRegistry creates a new ToolRegistry
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		Tools:     make(map[string]Tool),
		Executors: make(map[string]ToolExecutor),
	}
}

// RegisterExecutor registers a tool executor (mcp or bash)
func (r *ToolRegistry) RegisterExecutor(executorType string, executor ToolExecutor) {
	r.Executors[executorType] = executor
}

// Register registers a tool
func (r *ToolRegistry) Register(tool Tool) {
	r.Tools[tool.Name] = tool
}

// Execute executes a tool by name (supports fuzzy matching)
func (r *ToolRegistry) Execute(name string, args map[string]interface{}) (string, error) {
	// Try exact match first
	tool, ok := r.Tools[name]
	if !ok {
		// Try fuzzy match: find tool where name ends with the same suffix
		// e.g., "fetch:fetch_url" matches "fetch:fetch"
		for registeredName, t := range r.Tools {
			if len(name) > len(registeredName) && name[len(name)-len(registeredName):] == registeredName {
				tool = t
				name = registeredName // Use registered name for executor
				ok = true
				break
			}
			// Also try prefix match
			if len(registeredName) > len(name) && registeredName[len(registeredName)-len(name):] == name {
				tool = t
				ok = true
				break
			}
		}
	}

	if !ok {
		return "", fmt.Errorf("tool not found: %s", name)
	}

	executor, ok := r.Executors[tool.Executor]
	if !ok {
		return "", fmt.Errorf("executor not found: %s", tool.Executor)
	}

	return executor.ExecuteTool(name, args)
}

// GetToolsPrompt returns the tools prompt for LLM
func (r *ToolRegistry) GetToolsPrompt() string {
	if len(r.Tools) == 0 {
		return ""
	}

	var prompt string
	prompt = "## 可用工具\n\n"
	prompt += "你可以通过 function calling 来调用以下工具：\n\n"

	for _, tool := range r.Tools {
		prompt += fmt.Sprintf("- **%s**: %s\n", tool.Name, tool.Description)
	}

	return prompt
}

// GetTool returns a tool by name
func (r *ToolRegistry) GetTool(name string) (Tool, bool) {
	tool, ok := r.Tools[name]
	return tool, ok
}

// ListTools returns all registered tools
func (r *ToolRegistry) ListTools() []Tool {
	tools := make([]Tool, 0, len(r.Tools))
	for _, tool := range r.Tools {
		tools = append(tools, tool)
	}
	return tools
}
