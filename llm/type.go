package llm

import (
	"encoding/json"
)

type LLM interface {
	RunInference(messages []Message, tools []ToolDefinition) ([]Message, error)
}

type Message struct {
	Role       Role
	Text       string
	ToolResult *ToolResult
	ToolUse    *ToolUse
}

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)

type ToolUse struct {
	ID    string
	Name  string
	Input json.RawMessage
}

type ToolResult struct {
	ID      string
	Content string
	IsError bool
}

type ToolDefinition struct {
	Name                string
	Description         string
	InputSchemaInstance interface{}
	Func                func(json.RawMessage) (string, error)
}
