package llm

import (
	"encoding/json"
)

type LLM interface {
	RunInference(messages []Message, tools []ToolDefinition) ([]Message, error)
}

type MessageType string

const (
	MessageTypeText       MessageType = "text"
	MessageTypeToolUse    MessageType = "tool_use"
	MessageTypeToolResult MessageType = "tool_result"
)

type Message struct {
	Type       MessageType
	Role       Role
	Text       string
	ToolResult *ToolResult
	ToolUse    *ToolUse
}

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type ToolUse struct {
	ID    string
	Name  string
	Input json.RawMessage
	// required by google
	ThoughtSignature []byte
}

type ToolResult struct {
	ID       string
	ToolName string
	Content  string
	IsError  bool
}

type ToolDefinition struct {
	Name                string
	Description         string
	InputSchemaInstance interface{}
	Func                func(json.RawMessage) (string, error)
}
