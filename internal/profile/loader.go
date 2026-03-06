package profile

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Param YAML 参数定义
type Param struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Required bool   `yaml:"required"`
	Desc     string `yaml:"desc"`
}

// Capability YAML 能力定义
type Capability struct {
	Name        string  `yaml:"name"`
	Description string  `yaml:"description"`
	Inputs      []Param `yaml:"inputs,omitempty"`
	Outputs     []Param `yaml:"outputs,omitempty"`
	EstTime     string  `yaml:"est_time,omitempty"`
}

// Profile PROFILE.md 结构
type Profile struct {
	Name         string       `yaml:"name"`
	Role         string       `yaml:"role"`
	Version      string       `yaml:"version"`
	Description  string       `yaml:"description"`
	Capabilities []Capability `yaml:"capabilities"`

	// 工具声明（声明式权限）
	Tools       ToolsConfig `yaml:"tools,omitempty"`
	Constraints []string    `yaml:"constraints,omitempty"`

	// Markdown 正文（--- 之后的内容）
	Body string `yaml:"-"`
}

// ToolsConfig 工具配置
type ToolsConfig struct {
	// 是否启用 bash 工具
	Bash *BashConfig `yaml:"bash,omitempty"`
	// 其他工具（预留）
	Git    *GitConfig    `yaml:"git,omitempty"`
	Docker *DockerConfig `yaml:"docker,omitempty"`
}

// BashConfig Bash 工具配置
type BashConfig struct {
	Enabled   bool     `yaml:"enabled"`
	Allow     []string `yaml:"allow,omitempty"`      // 允许的命令
	Deny      []string `yaml:"deny,omitempty"`       // 禁止的命令
	Timeout   string   `yaml:"timeout,omitempty"`    // 超时
	MaxOutput int      `yaml:"max_output,omitempty"` // 最大输出
}

// GitConfig Git 工具配置
type GitConfig struct {
	Enabled bool     `yaml:"enabled"`
	Allow   []string `yaml:"allow,omitempty"` // 允许的 git 子命令
}

// DockerConfig Docker 工具配置
type DockerConfig struct {
	Enabled bool `yaml:"enabled"`
}

// Load 从文件加载 Profile
func Load(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)

	// 检查是否有 YAML Front Matter
	if !strings.HasPrefix(content, "---") {
		// 纯 Markdown，返回空 Profile + 全文作为 Body
		return &Profile{Body: content}, nil
	}

	// 提取 YAML 部分（两个 --- 之间）
	rest := content[4:] // 跳过第一个 "---"
	end := strings.Index(rest, "\n---")
	if end == -1 {
		return nil, fmt.Errorf("invalid profile format: missing closing ---")
	}

	yamlData := rest[:end]
	body := strings.TrimSpace(rest[end+4:]) // 跳过第二个 "---"

	var p Profile
	if err := yaml.Unmarshal([]byte(yamlData), &p); err != nil {
		return nil, err
	}

	// 验证 Profile 合法性
	if err := p.validate(); err != nil {
		return nil, err
	}

	p.Body = body
	return &p, nil
}

// validate 验证 Profile 配置
func (p *Profile) validate() error {
	if p.Name == "" {
		return fmt.Errorf("profile name is required")
	}
	if p.Role == "" {
		return fmt.Errorf("profile role is required")
	}

	// 检查 Allow 和 Deny 是否有重复命令
	if p.Tools.Bash != nil && p.Tools.Bash.Enabled {
		allowSet := make(map[string]bool)
		for _, cmd := range p.Tools.Bash.Allow {
			if allowSet[cmd] {
				return fmt.Errorf("duplicate command in allow list: %s", cmd)
			}
			allowSet[cmd] = true
		}
		for _, cmd := range p.Tools.Bash.Deny {
			if allowSet[cmd] {
				return fmt.Errorf("command %s appears in both allow and deny lists", cmd)
			}
		}
	}

	return nil
}

// BuildSystemPrompt 构建 LLM system prompt
func (p *Profile) BuildSystemPrompt(taskType string) string {
	var sb strings.Builder

	// 1. 基础描述
	if p.Description != "" {
		sb.WriteString("你是 ")
		sb.WriteString(p.Description)
		sb.WriteString("\n\n")
	}

	// 2. Markdown 正文
	if p.Body != "" {
		sb.WriteString(p.Body)
		sb.WriteString("\n\n")
	}

	// 3. 可用工具（关键：告诉 LLM 它能做什么）
	sb.WriteString("---\n")
	sb.WriteString("可用工具\n\n")

	// Bash 工具
	if p.Tools.Bash != nil && p.Tools.Bash.Enabled {
		sb.WriteString("✅ 你已启用 Bash 工具，可以执行以下命令：\n")
		if len(p.Tools.Bash.Allow) > 0 {
			sb.WriteString("允许命令：")
			sb.WriteString(strings.Join(p.Tools.Bash.Allow, ", "))
			sb.WriteString("\n")
		}
		if len(p.Tools.Bash.Deny) > 0 {
			sb.WriteString("禁止命令：")
			sb.WriteString(strings.Join(p.Tools.Bash.Deny, ", "))
			sb.WriteString("\n")
		}
		sb.WriteString("提示：你可以在代码生成后使用这些命令创建目录、编译代码、执行测试等。\n\n")
	} else {
		sb.WriteString("❌ 你没有 Bash 工具权限，只能输出文本内容。\n\n")
	}

	// Git 工具
	if p.Tools.Git != nil && p.Tools.Git.Enabled {
		sb.WriteString("✅ 你已启用 Git 工具\n")
		if len(p.Tools.Git.Allow) > 0 {
			sb.WriteString("允许操作：")
			sb.WriteString(strings.Join(p.Tools.Git.Allow, ", "))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// 4. 当前任务
	sb.WriteString("---\n")
	sb.WriteString("当前任务：")
	sb.WriteString(taskType)
	sb.WriteString("\n\n请根据你的角色定义完成任务。")

	// 5. 输出格式提示
	sb.WriteString("\n\n")
	sb.WriteString("输出格式要求：\n")
	sb.WriteString("请直接输出内容。如果需要执行命令，请先生成命令内容，然后说明应该执行什么命令。\n")

	return sb.String()
}
