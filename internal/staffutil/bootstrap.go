package staffutil

import (
	"cyberteam/internal/agent"
	"cyberteam/internal/llm"
	"cyberteam/internal/profile"
	"cyberteam/internal/protocol"
	"cyberteam/internal/tools"
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
	Memory    agent.Memory
	Debug     bool
}

// Debugf prints a debug message to stderr if debug mode is enabled.
func (s *StaffConfig) Debugf(format string, args ...interface{}) {
	if s.Debug {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

// ParseFlags parses common staff CLI flags and returns the config.
// roleName is used in the usage message (e.g. "product", "developer", "tester").
func ParseFlags(roleName string) *StaffConfig {
	apiKey := os.Getenv("OPENAI_API_KEY")
	baseURL := GetEnv("OPENAI_BASE_URL", "https://api.openai.com/v1")
	model := GetEnv("OPENAI_MODEL", "gpt-4o")

	var (
		id    = flag.String("id", "", "Staff ID")
		name  = flag.String("name", "", "Staff name")
		debug = flag.Bool("debug", false, "Enable debug logging")
	)
	flag.Parse()

	if *id == "" || *name == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s --id <id> --name <name>\n", roleName)
		os.Exit(1)
	}

	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "错误: 未设置 OPENAI_API_KEY 环境变量\n")
		fmt.Fprintf(os.Stderr, "请设置 API Key 后重试:\n")
		fmt.Fprintf(os.Stderr, "  export OPENAI_API_KEY=your-api-key\n")
		os.Exit(1)
	}

	return &StaffConfig{
		ID:        *id,
		Name:      *name,
		APIKey:    apiKey,
		BaseURL:   baseURL,
		Model:     model,
		LLMClient: llm.NewOpenAIClient(apiKey, baseURL),
		Debug:     *debug,
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

// LoadMemory loads the MEMORY.md from the executable's directory.
// It creates a FileMemory with personal and optional shared memory paths.
func (c *StaffConfig) LoadMemory(sharedPath string) {
	execPath, _ := os.Executable()
	execDir := filepath.Dir(execPath)
	memoryPath := filepath.Join(execDir, "MEMORY.md")

	// Create FileMemory with personal and shared paths
	m := agent.NewFileMemoryWithPaths(memoryPath, sharedPath)
	c.Memory = m
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
func (c *StaffConfig) SetupWorker(role string, handler worker.Handler) *worker.BaseWorker {
	profileData := c.BuildWorkerProfile(role)
	bw := worker.NewBaseWorker(profileData, handler)

	participant := NewMeetingParticipant(role, c.Name, c.Profile, c.LLMClient, c.Model, c.Memory, c.Debug)

	// 直接连接 MCP Server
	mcpClient, err := NewStaffMCPClient(GetEnv("MCP_CONFIG_PATH", "config/mcp.yaml"), role)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[Staff] 初始化 MCP 客户端失败: %v\n", err)
	} else if mcpClient != nil {
		participant.MCPClient = mcpClient
	}

	// 初始化 Bash 工具
	if c.Profile.Tools.Bash != nil && c.Profile.Tools.Bash.Enabled {
		bashTool := tools.NewBashToolWithLists(".", c.Profile.Tools.Bash.Allow, c.Profile.Tools.Bash.Deny)
		participant.BashTool = bashTool
		fmt.Fprintf(os.Stderr, "[Staff] Bash 工具已启用, allow=%d deny=%d\n",
			len(c.Profile.Tools.Bash.Allow), len(c.Profile.Tools.Bash.Deny))
	}

	bw.SetMeetingHandler(&GenericMeetingHandler{Participant: participant})
	bw.SetPrivateHandler(&GenericPrivateHandler{Participant: participant})

	return bw
}
