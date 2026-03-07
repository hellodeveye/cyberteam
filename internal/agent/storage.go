package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Storage handles agent persistence
type Storage struct {
	basePath string
}

// NewStorage creates a new Storage instance
func NewStorage(basePath string) *Storage {
	return &Storage{
		basePath: basePath,
	}
}

// EnsureDir ensures the storage directory exists
func (s *Storage) EnsureDir() error {
	return os.MkdirAll(s.basePath, 0755)
}

// SaveAgent saves agent state to file
func (s *Storage) SaveAgent(agent *Agent) error {
	if err := s.EnsureDir(); err != nil {
		return fmt.Errorf("ensure directory: %w", err)
	}

	state := AgentState{
		ID:           agent.config.ID,
		Name:         agent.config.Name,
		Role:         agent.config.Role,
		SystemPrompt: agent.config.SystemPrompt,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	path := filepath.Join(s.basePath, fmt.Sprintf("%s.json", agent.config.ID))
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	// Save memory
	memPath := filepath.Join(s.basePath, fmt.Sprintf("%s_memory.json", agent.config.ID))
	if err := agent.memory.Save(memPath); err != nil {
		return fmt.Errorf("save memory: %w", err)
	}

	return nil
}

// LoadAgent loads agent state from file
func (s *Storage) LoadAgent(agentID string) (*AgentState, error) {
	path := filepath.Join(s.basePath, fmt.Sprintf("%s.json", agentID))

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No saved state
		}
		return nil, fmt.Errorf("read file: %w", err)
	}

	var state AgentState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}

	return &state, nil
}

// LoadMemory loads agent memory from file
func (s *Storage) LoadMemory(agentID string, memory Memory) error {
	memPath := filepath.Join(s.basePath, fmt.Sprintf("%s_memory.json", agentID))
	return memory.Load(memPath)
}

// DeleteAgent deletes agent state files
func (s *Storage) DeleteAgent(agentID string) error {
	// Delete agent state
	agentPath := filepath.Join(s.basePath, fmt.Sprintf("%s.json", agentID))
	if err := os.Remove(agentPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove agent file: %w", err)
	}

	// Delete memory
	memPath := filepath.Join(s.basePath, fmt.Sprintf("%s_memory.json", agentID))
	if err := os.Remove(memPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove memory file: %w", err)
	}

	return nil
}

// AgentState represents the persisted agent state
type AgentState struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	SystemPrompt string `json:"system_prompt"`
}
