package staffutil

import (
	"cyberteam/internal/llm"
	"cyberteam/internal/profile"
	"cyberteam/internal/protocol"
	"cyberteam/internal/worker"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// StaffConfig holds the parsed configuration for a staff process.
type StaffConfig struct {
	ID        string
	Name      string
	APIKey    string
	BaseURL   string
	Model     string
	LLMClient llm.Client
	Profile   *profile.Profile
	MCPClient *MCPClient // MCP 客户端（SetupWorker 后可用）
}

// ParseFlags parses common staff CLI flags and returns the config.
// roleName is used in the usage message (e.g. "product", "developer", "tester").
func ParseFlags(roleName string) *StaffConfig {
	var (
		id      = flag.String("id", "", "Staff ID")
		name    = flag.String("name", "", "Staff name")
		apiKey  = flag.String("api-key", os.Getenv("OPENAI_API_KEY"), "OpenAI API Key")
		baseURL = flag.String("base-url", GetEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"), "OpenAI Base URL")
		model   = flag.String("model", GetEnv("OPENAI_MODEL", "gpt-4o"), "LLM Model")
	)
	flag.Parse()

	if *id == "" || *name == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s --id <id> --name <name>\n", roleName)
		os.Exit(1)
	}

	if *apiKey == "" {
		fmt.Fprintf(os.Stderr, "错误: 未设置 OPENAI_API_KEY 环境变量\n")
		fmt.Fprintf(os.Stderr, "请设置 API Key 后重试:\n")
		fmt.Fprintf(os.Stderr, "  export OPENAI_API_KEY=your-api-key\n")
		os.Exit(1)
	}

	return &StaffConfig{
		ID:        *id,
		Name:      *name,
		APIKey:    *apiKey,
		BaseURL:   *baseURL,
		Model:     *model,
		LLMClient: llm.NewOpenAIClient(*apiKey, *baseURL),
	}
}

// LoadProfile loads the PROFILE.md from the executable's directory,
// falling back to defaultProfile if loading fails.
func (c *StaffConfig) LoadProfile(defaultProfile *profile.Profile) {
	execPath, _ := os.Executable()
	execDir := filepath.Dir(execPath)
	profilePath := filepath.Join(execDir, "PROFILE.md")

	if p, err := profile.Load(profilePath); err == nil {
		c.Profile = p
	} else {
		c.Profile = defaultProfile
	}
}

// BuildWorkerProfile creates a protocol.WorkerProfile from the config.
func (c *StaffConfig) BuildWorkerProfile(role string) *protocol.WorkerProfile {
	return &protocol.WorkerProfile{
		ID:              c.ID,
		Name:            c.Name,
		Role:            role,
		Version:         "1.0.0",
		Capabilities:    BuildCapabilities(c.Profile),
		Status:          protocol.StatusIdle,
		Load:            0,
		ProfileMarkdown: c.Profile.Body,
	}
}

// SetupWorker creates the BaseWorker and wires up meeting/private handlers.
// After calling this, c.MCPClient is available for task handlers.
func (c *StaffConfig) SetupWorker(role string, handler worker.Handler) *worker.BaseWorker {
	profileData := c.BuildWorkerProfile(role)
	bw := worker.NewBaseWorker(profileData, handler)

	mcpClient := NewMCPClient(bw.CallMCP)
	c.MCPClient = mcpClient // 保存供任务处理器使用

	participant := NewMeetingParticipant(role, c.Name, c.Profile, c.LLMClient, c.Model)
	participant.MCPClient = mcpClient

	bw.SetMeetingHandler(&GenericMeetingHandler{Participant: participant})
	bw.SetPrivateHandler(&GenericPrivateHandler{Participant: participant})

	return bw
}
